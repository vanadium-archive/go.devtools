// Package dashboard is an implementation of a Vanadium dashboard web
// server.
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
	"sort"
	"strings"
	"text/template"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/testutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/xunit"
)

const (
	defaultBucket = "gs://vanadium-test-results"
)

var (
	bucketFlag    string
	cacheFlag     string
	dryRunFlag    bool
	colorFlag     bool
	portFlag      int
	staticDirFlag string
	verboseFlag   bool
)

func init() {
	flag.StringVar(&bucketFlag, "bucket", defaultBucket, "Google Storage bucket to use for fetching files.")
	flag.StringVar(&cacheFlag, "cache", "", "Directory to use for caching files.")
	flag.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	flag.BoolVar(&colorFlag, "color", true, "Use color to format output.")
	flag.StringVar(&staticDirFlag, "static", "", "Directory to use for serving static files.")
	flag.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	flag.IntVar(&portFlag, "port", 8000, "Port for the server.")
	flag.Parse()
}

type summaryData struct {
	Number string
	OSJobs []osJobs
}

type osJobs struct {
	Jobs   []job
	OSName string
}

type job struct {
	Name           string
	Arch           string
	HasGo32BitTest bool
	Result         bool
	FailedTests    []failedTest
}

type failedTest struct {
	Suite     string
	ClassName string
	TestCase  string
}

var go32BitTests = map[string]struct{}{
	"vanadium-go-build": struct{}{},
	"vanadium-go-test":  struct{}{},
}

var summaryTemplate = template.Must(template.New("summary").Parse(`
{{ $n := .Number }}
<!DOCTYPE html>
<html>
<head>
	<title>Presubmit #{{ $n }} Summary</title>
	<link rel="stylesheet" href="/static/dashboard.css">
</head>
<body>
<h1>Presubmit #{{ $n }} Summary</h1>
<ul>
{{ range $osJobs := .OSJobs }}
	<h2 class="os-label">{{ $osJobs.OSName }}</h2>
	<ul class="job-list">
	{{ range $job := .Jobs }}
		<li>
		{{ if $job.Result }}
			<span class="label-pass">PASS</span>
		{{ else }}
			<span class="label-fail">FAIL</span>
		{{ end }}
		<a target="_blank" href="index.html?type=presubmit&n={{ $n }}&arch={{ $job.Arch }}&os={{ $osJobs.OSName }}&job={{ $job.Name }}">{{ $job.Name }}</a>
		{{ if $job.HasGo32BitTest }}
		<span class="label-arch">{{ $job.Arch }}</span>
		{{ end }}
		{{ if gt (len $job.FailedTests) 0 }}
			<ol class="test-list">
			{{ range $failedTest := $job.FailedTests }}
				<li>
					<a target="_blank" href="index.html?type=presubmit&n={{ $n }}&arch={{ $job.Arch }}&os={{ $osJobs.OSName }}&job={{ $job.Name }}&suite={{ $failedTest.Suite }}&class={{ $failedTest.ClassName }}&test={{ $failedTest.TestCase }}">{{ $failedTest.ClassName }}/{{ $failedTest.TestCase }}</a>
				</li>
			{{ end }}
			</ol>
		{{ end }}
		</li>
	{{ end }}
	</ul>
{{ end }}
</ul>
</body>
</html>
`))

type jobData struct {
	Job         string
	OSName      string
	Arch        string
	Number      string
	Output      string
	Result      bool
	FailedTests []failedTest
}

var jobTemplate = template.Must(template.New("job").Funcs(templateFuncMap).Parse(`
{{ $n := .Number }}
{{ $osName := .OSName }}
{{ $arch := .Arch }}
{{ $jobName := .Job }}
<!DOCTYPE html>
<html>
<head>
	<title>Presubmit #{{ $n }} Job Details</title>
	<link rel="stylesheet" href="/static/dashboard.css">
</head>
<body>
<h1>Presubmit #{{ $n }} Job Details</h1>
<table class="param-table">
	<tr><th class="param-table-name-col"></th><th></th></tr>
	<tr><td>OS</td><td>{{ $osName }}</td></tr>
	<tr><td>Arch</td><td>{{ $arch }}</td></tr>
	<tr><td>Job</td><td>{{ $jobName }}</td></tr>
</table>
{{ if .Result }}
<h2 class="label-pass-large">PASS</h2>
{{ else }}
<h2 class="label-fail-large">FAIL</h2>
<ol class="test-list2">
{{ range $failedTest := .FailedTests }}
	<li>
		<a target="_blank" href="index.html?type=presubmit&n={{ $n }}&arch={{ $arch }}&os={{ $osName }}&job={{ $jobName }}&suite={{ $failedTest.Suite }}&class={{ $failedTest.ClassName }}&test={{ $failedTest.TestCase }}">{{ $failedTest.ClassName }}/{{ $failedTest.TestCase }}</a>
	</li>
{{ end }}
</ol>
{{ end }}
<h2>Console Output:</h2>
<pre>{{ colors .Output }}</pre>
</body>
</html>
`))

type testData struct {
	Job      string
	OSName   string
	Arch     string
	Number   string
	TestCase xunit.TestCase
}

var testTemplate = template.Must(template.New("test").Funcs(templateFuncMap).Parse(`
<!DOCTYPE html>
<html>
<head>
	<title>Presubmit #{{ .Number }} Test Details</title>
	<link rel="stylesheet" href="/static/dashboard.css">
</head>
<body>
<h1>Presubmit #{{ .Number }} Test Details</h1>
<table class="param-table">
	<tr><th class="param-table-name-col"></th><th></th></tr>
	<tr><td>OS</td><td>{{ .OSName }}</td></tr>
	<tr><td>Arch</td><td>{{ .Arch }}</td></tr>
	<tr><td>Job</td><td>{{ .Job }}</td></tr>
	<tr><td>Suite</td><td>{{ .TestCase.Classname }}</td></tr>
	<tr><td>Test</td><td>{{ .TestCase.Name }}</td></tr>
</table>
{{ if eq (len .TestCase.Failures) 0 }}
<h2 class="label-pass-large">PASS</h2>
{{ else }}
<h2 class="label-fail-large">FAIL</h2>
<a target="_blank" href="index.html?type=presubmit&n={{ .Number}}&arch={{ .Arch }}&os={{ .OSName }}&job={{ .Job }}">Console Log</a>
<h2>Failures:</h2>
<ul>
	{{ range $failure := .TestCase.Failures }}
	<li> {{ $failure.Message }}: <br/>
  	<pre>{{ colors $failure.Data }}</pre>
	</li>
	{{ end }}
</ul>
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

type params struct {
	arch      string
	job       string
	testCase  string
	testClass string
	testSuite string
	osName    string
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

func displayPresubmitPage(ctx *tool.Context, w http.ResponseWriter, r *http.Request) (e error) {
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
	// The dir structure is:
	// <root>/presubmit/<n>/<os>/<arch>/<job>/...
	if err := ctx.Run().MkdirAll(filepath.Join(root, "presubmit"), os.FileMode(0700)); err != nil {
		return err
	}
	n := r.Form.Get("n")
	targetDir := filepath.Join(root, "presubmit", n)
	if _, err := os.Stat(targetDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat() failed: %v", err)
		}
		// To avoid interference between concurrent requests, download data to a
		// tmp dir, and move it to the final location.
		bucket := bucketFlag + "/v0/presubmit/" + n
		tmpDir, err := ctx.Run().TempDir(root, "")
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		if err := ctx.Run().Command("gsutil", "-m", "-q", "cp", "-r", bucket, tmpDir); err != nil {
			return err
		}
		if err := ctx.Run().Rename(filepath.Join(tmpDir, n), targetDir); err != nil {
			// If the target directory already exists, it must have been created by
			// a concurrent request.
			if !strings.Contains(err.Error(), "directory not empty") {
				return err
			}
		}
	}

	params := extractParams(r)
	switch {
	case params.arch == "" || params.osName == "" || params.job == "":
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
	case params.testSuite == "":
		// Generate the job detail page.
		path := filepath.Join(root, "presubmit", n, params.osName, params.arch, params.job)
		data, err := generateJobData(ctx, n, params.osName, params.arch, params.job, path)
		if err != nil {
			return err
		}
		if err := jobTemplate.Execute(w, data); err != nil {
			return fmt.Errorf("Execute() failed: %v", err)
		}
	case params.testClass != "" && params.testSuite != "" && params.testCase != "":
		// Generate the test detail page.
		path := filepath.Join(root, "presubmit", n, params.osName, params.arch, params.job)
		data, err := generateTestData(ctx, n, params.osName, params.arch, params.job, params.testSuite, params.testClass, params.testCase, path)
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

func extractParams(r *http.Request) params {
	return params{
		arch:      r.Form.Get("arch"),
		job:       r.Form.Get("job"),
		testCase:  r.Form.Get("test"),
		testClass: r.Form.Get("class"),
		testSuite: r.Form.Get("suite"),
		osName:    r.Form.Get("os"),
	}
}

func generateSummaryData(ctx *tool.Context, n, path string) (*summaryData, error) {
	data := summaryData{n, []osJobs{}}
	osFileInfos, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("ReadDir(%v) failed: %v", path, err)
	}
	for _, osFileInfo := range osFileInfos {
		osName := osFileInfo.Name()
		osDir := filepath.Join(path, osName)
		archFileInfos, err := ioutil.ReadDir(osDir)
		if err != nil {
			return nil, fmt.Errorf("ReadDir(%v) failed: %v", osDir, err)
		}
		o := osJobs{
			Jobs:   []job{},
			OSName: osName,
		}
		jobsMap := map[string]job{}
		jobKeys := []string{}
		for _, archFileInfo := range archFileInfos {
			arch := archFileInfo.Name()
			archDir := filepath.Join(osDir, arch)
			jobFileInfos, err := ioutil.ReadDir(archDir)
			if err != nil {
				return nil, fmt.Errorf("ReadDir(%v) failed: %v", archDir, err)
			}
			for _, jobFileInfo := range jobFileInfos {
				// Parse test result (pass/fail).
				jobName := jobFileInfo.Name()
				jobDir := filepath.Join(archDir, jobName)
				bytes, err := ctx.Run().ReadFile(filepath.Join(jobDir, "result"))
				if err != nil {
					return nil, err
				}
				var r testutil.TestResult
				if err := json.Unmarshal(bytes, &r); err != nil {
					return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
				}
				_, hasGo32BitTest := go32BitTests[jobName]
				j := job{
					Name:           jobName,
					Arch:           arch,
					HasGo32BitTest: hasGo32BitTest,
					Result:         r.Status == testutil.TestPassed,
					FailedTests:    []failedTest{},
				}
				jobKey := fmt.Sprintf("%s-%s", jobName, arch)
				jobKeys = append(jobKeys, jobKey)

				// Parse failed tests.
				failedTests, err := parseFailedTests(ctx, jobDir)
				if err != nil {
					return nil, err
				}
				j.FailedTests = append(j.FailedTests, failedTests...)
				jobsMap[jobKey] = j
			}
		}
		sort.Strings(jobKeys)
		for _, jobKey := range jobKeys {
			o.Jobs = append(o.Jobs, jobsMap[jobKey])
		}
		data.OSJobs = append(data.OSJobs, o)
	}
	return &data, nil
}

func generateJobData(ctx *tool.Context, n, osName, arch, job, path string) (*jobData, error) {
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
	failedTests, err := parseFailedTests(ctx, path)
	if err != nil {
		return nil, err
	}
	data := jobData{
		Job:         job,
		OSName:      osName,
		Arch:        arch,
		Number:      n,
		Output:      string(outputBytes),
		Result:      r.Status == testutil.TestPassed,
		FailedTests: failedTests,
	}
	return &data, nil
}

func generateTestData(ctx *tool.Context, n, osName, arch, job, testSuite, testClass, testCase, path string) (*testData, error) {
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
				if tc.Name == testCase && tc.Classname == testClass {
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
		OSName:   osName,
		Arch:     arch,
		Number:   n,
		TestCase: test,
	}
	return &data, nil
}

func parseFailedTests(ctx *tool.Context, jobDir string) ([]failedTest, error) {
	failedTests := []failedTest{}
	suitesBytes, err := ctx.Run().ReadFile(filepath.Join(jobDir, "xunit.xml"))
	if os.IsNotExist(err) {
		return failedTests, nil
	}
	if err != nil {
		return nil, err
	}

	var s xunit.TestSuites
	if err := xml.Unmarshal(suitesBytes, &s); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(suitesBytes), err)
	}
	for _, ts := range s.Suites {
		for _, tc := range ts.Cases {
			if len(tc.Failures) > 0 || len(tc.Errors) > 0 {
				failedTests = append(failedTests, failedTest{
					Suite:     ts.Name,
					ClassName: tc.Classname,
					TestCase:  tc.Name,
				})
			}
		}
	}
	return failedTests, nil
}

func validateValues(values url.Values) error {
	ty := values.Get("type")
	if ty == "" {
		return fmt.Errorf("required parameter 'type' not found")
	}
	if ty == "presubmit" {
		paramsToCheck := []string{}
		if n := values.Get("n"); n == "" {
			return fmt.Errorf("required parameter 'n' not found")
		} else {
			paramsToCheck = append(paramsToCheck, "n")
		}
		paramsToCheck = append(paramsToCheck, "job", "os", "arch", "suite", "test")
		if err := checkPathTraversal(values, paramsToCheck); err != nil {
			return err
		}
	}
	return nil
}

func checkPathTraversal(values url.Values, params []string) error {
	for _, param := range params {
		if value := values.Get(param); value != "" && strings.Contains(value, "..") {
			return fmt.Errorf("parameter %q is not allowed to contain '..'", param)
		}
	}
	return nil
}

func helper(ctx *tool.Context, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if err := validateValues(r.Form); err != nil {
		fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		http.Error(w, "500 internal server error", http.StatusInternalServerError)
		return
	}

	switch r.Form.Get("type") {
	case "presubmit":
		if err := displayPresubmitPage(ctx, w, r); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
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

func loggingHandler(ctx *tool.Context, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(ctx.Stdout(), "%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func main() {
	ctx := tool.NewContext(tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	handler := func(w http.ResponseWriter, r *http.Request) {
		helper(ctx, w, r)
	}
	health := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	staticHandler := http.FileServer(http.Dir(staticDirFlag))
	http.Handle("/static/", http.StripPrefix("/static/", staticHandler))
	http.Handle("/favicon.ico", staticHandler)
	http.HandleFunc("/health", health)
	http.HandleFunc("/", handler)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", portFlag), loggingHandler(ctx, http.DefaultServeMux)); err != nil {
		fmt.Fprintf(os.Stderr, "ListenAndServer() failed: %v", err)
		os.Exit(1)
	}
}
