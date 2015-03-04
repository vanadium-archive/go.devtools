// Package build is an implementation of a web server for displaying
// test results.
package main

// TODO(jsimsa): Move style formatting to a .css file.

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"v.io/x/devtools/lib/collect"
	"v.io/x/devtools/lib/testutil"
	"v.io/x/devtools/lib/util"
	"v.io/x/devtools/lib/xunit"
)

const (
	defaultBucket = "gs://vanadium-test-results"
)

var (
	bucketFlag  string
	cacheFlag   string
	dryRunFlag  bool
	noColorFlag bool
	portFlag    int
	verboseFlag bool
)

func init() {
	flag.StringVar(&bucketFlag, "bucket", defaultBucket, "Google Storage bucket to use for fetching files.")
	flag.StringVar(&cacheFlag, "cache", "", "Directory to use for caching files.")
	flag.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	flag.BoolVar(&noColorFlag, "nocolor", false, "Do not use color to format output.")
	flag.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	flag.IntVar(&portFlag, "port", 8000, "Port for the server.")
	flag.Parse()
}

type summaryData struct {
	Number string
	Labels []label
}

type label struct {
	Jobs []job
	Name string
}

type job struct {
	Name   string
	Result bool
}

var summaryTemplate = template.Must(template.New("summary").Parse(`
{{ $n := .Number }}
<html>
<head><title>Presubmit #{{ $n }} Summary</title></head>
<body>
<h1>Presubmit #{{ $n }}</h1>
<ul>
{{ range $label := .Labels }}
<h2>{{ $label.Name }}</h2>
<ul>
{{ range $job := .Jobs }}
<li>
{{ if $job.Result }}
<font color="green">PASS</font>
{{ else }}
<font color="red">FAIL</font>
{{ end }}
<a target="_blank" href="index.html?type=presubmit&n={{ $n }}&label={{ $label.Name }}&job={{ $job.Name }}>{{ $job.Name }}"</a>
{{ end }}
</ul>
{{ end }}
</body>
</html>
`))

type jobData struct {
	Job    string
	Label  string
	Number string
	Output string
	Result bool
}

var jobTemplate = template.Must(template.New("job").Funcs(templateFuncMap).Parse(`
<html>
<head><title>Presubmit #{{ .Number }} Job Details</title></head>
<body>
<h1>Presubmit #{{ .Number }} (label={{ .Label }}, job={{ .Job }})</h1>
{{ if .Result }}
<h2 style="color:green">PASS</h2>
{{ else }}
<h2 style="color:red">FAIL</h2>
{{ end }}
<h2>Console Output:</h2>
<pre>{{ colors .Output }}</pre>
</body>
</html>
`))

type testData struct {
	Job      string
	Label    string
	Number   string
	TestCase xunit.TestCase
}

var testTemplate = template.Must(template.New("test").Funcs(templateFuncMap).Parse(`
<html>
<head><title>Presubmit #{{ .Number }} Test Details</title></head>
<body>
<h1>Presubmit #{{ .Number }} (label={{ .Label }}, job={{ .Job }}, suite={{ .TestCase.Classname }}, test={{ .TestCase.Name }})</h1>
{{ if eq (len .TestCase.Failures) 0 }}
<h2 style="color:green">PASS</h2>
{{ else }}
<h2 style="color:red">FAIL</h2>
<h2>Failures:</h2>
<ul>
{{ range $failure := .TestCase.Failures }}
<li> {{ $failure.Message }}: <br/> <pre>{{ colors $failure.Data }}</pre>
{{ end }}
<ul>
{{ end }}
</body>
</html>
`))

var templateFuncMap = template.FuncMap{
	"colors": ansiColorsToHTML,
}

type ansiColor struct {
	code  string
	style string
}

var (
	ansiColors = []ansiColor{
		ansiColor{"30", "color:black"},
		ansiColor{"31", "color:red"},
		ansiColor{"32", "color:green"},
		ansiColor{"33", "color:yellow"},
		ansiColor{"34", "color:blue"},
		ansiColor{"35", "color:magenta"},
		ansiColor{"36", "color:cyan"},
		ansiColor{"37", "color:black"},
		ansiColor{"40", "background-color:black"},
		ansiColor{"41", "background-color:red"},
		ansiColor{"42", "background-color:green"},
		ansiColor{"43", "background-color:yellow"},
		ansiColor{"44", "background-color:blue"},
		ansiColor{"45", "background-color:magenta"},
		ansiColor{"46", "background-color:cyan"},
		ansiColor{"47", "background-color:black"},
	}
)

func ansiColorsToHTML(text string) (string, error) {
	for _, ansi := range ansiColors {
		re, err := regexp.Compile(fmt.Sprintf(`\[0;%sm(.*)\[0m`, ansi.code))
		if err != nil {
			return "", err
		}
		text = re.ReplaceAllString(text, fmt.Sprintf(`<font style="%s">$1</font>`, ansi.style))
	}
	return text, nil
}

func displayPresubmitPage(ctx *util.Context, w http.ResponseWriter, r *http.Request) (e error) {
	// Set up the root directory.
	root := cacheFlag
	if root == "" {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		root = tmpDir
	}

	// Fetch the presubmit test results.
	if err := ctx.Run().MkdirAll(filepath.Join(root, "presubmit"), os.FileMode(0700)); err != nil {
		return err
	}
	n := r.Form.Get("n")
	if _, err := os.Stat(filepath.Join(root, "presubmit", n)); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat() failed: %v", err)
		}
		bucket := bucketFlag + "/v0/presubmit/" + n
		if err := ctx.Run().Command("gsutil", "-m", "-q", "cp", "-r", bucket, filepath.Join(root, "presubmit")); err != nil {
			return err
		}
	}

	label, job, testSuite, testCase := r.Form.Get("label"), r.Form.Get("job"), r.Form.Get("suite"), r.Form.Get("test")
	switch {
	case label == "" || job == "":
		// Generate the summary page.
		path := filepath.Join(root, "presubmit", n)
		data, err := generateSummaryData(ctx, n, path)
		if err != nil {
			return err
		}
		if err := summaryTemplate.Execute(w, data); err != nil {
			return fmt.Errorf("Execute() failed: %v", err)
		}
		return nil
	case testSuite == "":
		// Generate the job detail page.
		path := filepath.Join(root, "presubmit", n, label, job)
		data, err := generateJobData(ctx, n, label, job, path)
		if err != nil {
			return err
		}
		if err := jobTemplate.Execute(w, data); err != nil {
			return fmt.Errorf("Execute() failed: %v", err)
		}
	case testSuite != "" && testCase != "":
		// Generate the test detail page.
		path := filepath.Join(root, "presubmit", n, label, job)
		data, err := generateTestData(ctx, n, label, job, testSuite, testCase, path)
		if err != nil {
			return err
		}
		if err := testTemplate.Execute(w, data); err != nil {
			return fmt.Errorf("Execute() failed: %v", err)
		}
	default:
		return fmt.Errorf("invalid combination of parameters")
	}
	return nil
}

func generateSummaryData(ctx *util.Context, n, path string) (*summaryData, error) {
	data := summaryData{n, []label{}}
	fileInfos, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("ReadDir(%v) failed: %v", path, err)
	}
	for _, fileInfo := range fileInfos {
		labelDir := filepath.Join(path, fileInfo.Name())
		fileInfos, err := ioutil.ReadDir(labelDir)
		if err != nil {
			return nil, fmt.Errorf("ReadDir(%v) failed: %v", path, err)
		}
		l := label{
			Jobs: []job{},
			Name: fileInfo.Name(),
		}
		for _, fileInfo := range fileInfos {
			jobDir := filepath.Join(labelDir, fileInfo.Name())
			bytes, err := ctx.Run().ReadFile(filepath.Join(jobDir, "result"))
			if err != nil {
				return nil, err
			}
			var r testutil.TestResult
			if err := json.Unmarshal(bytes, &r); err != nil {
				return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
			}
			j := job{
				Name:   fileInfo.Name(),
				Result: r.Status == testutil.TestPassed,
			}
			l.Jobs = append(l.Jobs, j)
		}
		data.Labels = append(data.Labels, l)
	}
	return &data, nil
}

func generateJobData(ctx *util.Context, n, label, job, path string) (*jobData, error) {
	outputBytes, err := ctx.Run().ReadFile(filepath.Join(path, "output"))
	if err != nil {
		return nil, err
	}
	resultBytes, err := ctx.Run().ReadFile(filepath.Join(path, "result"))
	if err != nil {
		return nil, err
	}
	var r testutil.TestResult
	if err := json.Unmarshal(resultBytes, &r); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(resultBytes), err)
	}
	data := jobData{
		Job:    job,
		Label:  label,
		Number: n,
		Output: string(outputBytes),
		Result: r.Status == testutil.TestPassed,
	}
	return &data, nil
}

func generateTestData(ctx *util.Context, n, label, job, testSuite, testCase, path string) (*testData, error) {
	suitesBytes, err := ctx.Run().ReadFile(filepath.Join(path, "xunit.xml"))
	if err != nil {
		return nil, err
	}
	var s xunit.TestSuites
	if err := xml.Unmarshal(suitesBytes, &s); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(suitesBytes), err)
	}
	var test xunit.TestCase
	found := false
outer:
	for _, ts := range s.Suites {
		if ts.Name == testSuite {
			for _, tc := range ts.Cases {
				if tc.Name == testCase {
					test = tc
					found = true
					break outer
				}
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("failed to find the test %s in test suite %s", testCase, testSuite)
	}
	data := testData{
		Job:      job,
		Label:    label,
		Number:   n,
		TestCase: test,
	}
	return &data, nil
}

func validateValues(values url.Values) error {
	ty := values.Get("type")
	if ty == "" {
		return fmt.Errorf("required parameter 'type' not found")
	}
	if ty == "presubmit" {
		if n := values.Get("n"); n == "" {
			return fmt.Errorf("required parameter 'n' not found")
		} else {
			if strings.Contains(n, "..") {
				return fmt.Errorf("parameter 'n' is not allowed to contain '..'")
			}
		}
		if job := values.Get("job"); job != "" && strings.Contains(job, "..") {
			return fmt.Errorf("parameter 'job' is not allowed to contain '..'")
		}
		if label := values.Get("label"); label != "" && strings.Contains(label, "..") {
			return fmt.Errorf("parameter 'label' is not allowed to contain '..'")
		}
		if suite := values.Get("suite"); suite != "" && strings.Contains(suite, "..") {
			return fmt.Errorf("parameter 'suite' is not allowed to contain '..'")
		}
		if test := values.Get("test"); test != "" && strings.Contains(test, "..") {
			return fmt.Errorf("parameter 'test' is not allowed to contain '..'")
		}
	}
	return nil
}

func helper(ctx *util.Context, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if err := validateValues(r.Form); err != nil {
		fmt.Fprintf(ctx.Stderr(), "%v", err)
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
		return
	}

	switch r.Form.Get("type") {
	case "presubmit":
		if err := displayPresubmitPage(ctx, w, r); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v", err)
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
		}
		// The presubmit test results data never changes, cache it in
		// the clients for up to 30 days.
		w.Header().Set("Cache-control", "public, max-age=2592000")
	default:
		fmt.Fprintf(ctx.Stderr(), "unknown type: %v", r.Form.Get("type"))
		http.NotFound(w, r)
	}
}

func main() {
	ctx := util.NewContext(nil, os.Stdin, os.Stdout, os.Stderr, !noColorFlag, dryRunFlag, verboseFlag)
	handler := func(w http.ResponseWriter, r *http.Request) {
		helper(ctx, w, r)
	}
	http.HandleFunc("/", handler)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", portFlag), nil); err != nil {
		fmt.Fprintf(os.Stderr, "ListenAndServer() failed: %v", err)
		os.Exit(1)
	}
}
