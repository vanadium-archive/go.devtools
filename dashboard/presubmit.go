// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/x/devtools/internal/cache"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

const (
	resultsBucket       = "gs://vanadium-test-results"
	multiPartOutputNote = `
###################################################################################
THIS TEST HAS BEEN DIVIDED INTO MULTIPLE PARTS THAT HAVE BEEN EXECUTED IN PARALLEL.
THE OUTPUT BELOW IS THE CONCATENATION OF THOSE OUTPUTS.
###################################################################################
`
)

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
	PartIndex      int
	HasGo32BitTest bool
	Result         bool
	FailedTests    []failedTest
}

type failedTest struct {
	Suite     string
	ClassName string
	TestCase  string
	PartIndex int
}

type aggregatedPartsData struct {
	result      bool
	failedTests []failedTest
	output      string
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
					<a target="_blank" href="index.html?type=presubmit&n={{ $n }}&arch={{ $job.Arch }}&os={{ $osJobs.OSName }}&job={{ $job.Name }}&part={{ $failedTest.PartIndex }}&suite={{ $failedTest.Suite }}&class={{ $failedTest.ClassName }}&test={{ $failedTest.TestCase }}">{{ $failedTest.ClassName }}/{{ $failedTest.TestCase }}</a>
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
	PartIndex   string
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
<br>
<a href="index.html?type=presubmit&n={{ $n }}">Back to Summary</a>
{{ if .Result }}
<h2 class="label-pass-large">PASS</h2>
{{ else }}
<h2 class="label-fail-large">FAIL</h2>
<ol class="test-list2">
{{ range $failedTest := .FailedTests }}
	<li>
		<a target="_blank" href="index.html?type=presubmit&n={{ $n }}&arch={{ $arch }}&os={{ $osName }}&job={{ $jobName }}&part={{ $failedTest.PartIndex }}&suite={{ $failedTest.Suite }}&class={{ $failedTest.ClassName }}&test={{ $failedTest.TestCase }}">{{ $failedTest.ClassName }}/{{ $failedTest.TestCase }}</a>
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
{{ $n := .Number }}
<!DOCTYPE html>
<html>
<head>
	<title>Presubmit #{{ $n }} Test Details</title>
	<link rel="stylesheet" href="/static/dashboard.css">
</head>
<body>
<h1>Presubmit #{{ $n }} Test Details</h1>
<table class="param-table">
	<tr><th class="param-table-name-col"></th><th></th></tr>
	<tr><td>OS</td><td>{{ .OSName }}</td></tr>
	<tr><td>Arch</td><td>{{ .Arch }}</td></tr>
	<tr><td>Job</td><td>{{ .Job }}</td></tr>
	<tr><td>Suite</td><td>{{ .TestCase.Classname }}</td></tr>
	<tr><td>Test</td><td>{{ .TestCase.Name }}</td></tr>
</table>
<br>
<a href="index.html?type=presubmit&n={{ $n }}">Back to Summary</a>
<br>
<a target="_blank" href="index.html?type=presubmit&n={{ .Number}}&arch={{ .Arch }}&os={{ .OSName }}&job={{ .Job }}">Console Log</a>
{{ if eq (len .TestCase.Failures) 0 }}
<h2 class="label-pass-large">PASS</h2>
{{ else }}
<h2 class="label-fail-large">FAIL</h2>
<h2>Failures:</h2>
<ul>
	{{ range $failure := .TestCase.Failures }}
	{{ if $failure.Message }}
	<li> {{ $failure.Message }}: <br/>
	{{ else }}
	<li> Failure: <br/>
	{{ end }}
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
	osName    string
	partIndex string
	testCase  string
	testClass string
	testSuite string
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
	escapedText := html.EscapeString(text)
	for _, ansi := range ansiColors {
		re, err := regexp.Compile(fmt.Sprintf(`\[0;%sm(.*)\[0m`, ansi.code))
		if err != nil {
			return "", err
		}
		escapedText = re.ReplaceAllString(escapedText, fmt.Sprintf(`<font style="%s">$1</font>`, ansi.style))
	}
	return escapedText, nil
}

func displayPresubmitPage(jirix *jiri.X, w http.ResponseWriter, r *http.Request) (e error) {
	// Set up the root directory.
	root := cacheFlag
	s := jirix.NewSeq()
	if root == "" {
		tmpDir, err := s.TempDir("", "")
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)
		root = tmpDir
	}

	// Fetch the presubmit test results.
	// The dir structure is:
	// <root>/presubmit/<n>/<os>/<arch>/<job>/<part>/...
	if err := s.MkdirAll(filepath.Join(root, "presubmit"), os.FileMode(0700)).Done(); err != nil {
		return err
	}
	n := r.Form.Get("n")
	_, err := cache.StoreGoogleStorageFile(jirix.Context, filepath.Join(root, "presubmit"), resultsBucketFlag+"/v0/presubmit", n)
	if err != nil {
		return err
	}

	params := extractParams(r)
	switch {
	case params.arch == "" || params.osName == "" || params.job == "":
		// Generate the summary page.
		path := filepath.Join(root, "presubmit", n)
		data, err := params.generateSummaryData(jirix, n, path)
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
		data, err := params.generateJobData(jirix, n, path)
		if err != nil {
			return err
		}
		if err := jobTemplate.Execute(w, data); err != nil {
			return fmt.Errorf("Execute() failed: %v", err)
		}
	case (params.testClass != "" || params.testSuite != "") && params.testCase != "":
		// Generate the test detail page.
		path := filepath.Join(root, "presubmit", n, params.osName, params.arch, params.job, params.partIndex)
		data, err := params.generateTestData(jirix, n, path)
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
		osName:    r.Form.Get("os"),
		partIndex: r.Form.Get("part"),
		testCase:  r.Form.Get("test"),
		testClass: r.Form.Get("class"),
		testSuite: r.Form.Get("suite"),
	}
}

func (p params) generateSummaryData(jirix *jiri.X, n, path string) (*summaryData, error) {
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
				jobName := jobFileInfo.Name()
				jobDir := filepath.Join(archDir, jobName)

				// Aggregate job data for all its parts.
				data, err := aggregateTestParts(jirix, jobDir, false)
				if err != nil {
					return nil, err
				}
				j := job{
					Name:           jobName,
					Arch:           arch,
					HasGo32BitTest: false,
					Result:         data.result,
					FailedTests:    data.failedTests,
				}
				if _, ok := go32BitTests[jobName]; ok {
					j.HasGo32BitTest = true
				}
				jobKey := fmt.Sprintf("%s-%s", jobName, arch)
				jobKeys = append(jobKeys, jobKey)
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

func (p params) generateJobData(jirix *jiri.X, n, path string) (*jobData, error) {
	data, err := aggregateTestParts(jirix, path, true)
	if err != nil {
		return nil, err
	}
	return &jobData{
		Job:         p.job,
		OSName:      p.osName,
		Arch:        p.arch,
		Number:      n,
		Result:      data.result,
		FailedTests: data.failedTests,
		Output:      data.output,
	}, nil
}

func (p params) generateTestData(jirix *jiri.X, n, path string) (*testData, error) {
	suitesBytes, err := jirix.NewSeq().ReadFile(filepath.Join(path, "xunit.xml"))
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
		if ts.Name == p.testSuite {
			for _, tc := range ts.Cases {
				if tc.Name == p.testCase && tc.Classname == p.testClass {
					test = tc
					if test.Classname == "" {
						test.Classname = ts.Name
					}
					found = true
					break outer
				}
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("failed to find the test %s in test suite %s", p.testCase, p.testSuite)
	}
	data := testData{
		Job:      p.job,
		OSName:   p.osName,
		Arch:     p.arch,
		Number:   n,
		TestCase: test,
	}
	return &data, nil
}

func aggregateTestParts(jirix *jiri.X, jobDir string, aggregateOutput bool) (*aggregatedPartsData, error) {
	// Read dirs for parts under the given job dir.
	partFileInfos, err := ioutil.ReadDir(jobDir)
	if err != nil {
		return nil, fmt.Errorf("ReadDir(%v) failed: %v", jobDir, err)
	}

	// Aggregate results and failed tests.
	data := &aggregatedPartsData{
		result: true,
	}
	outputs := []string{}
	s := jirix.NewSeq()
	for index, partFileInfo := range partFileInfos {
		part := partFileInfo.Name()
		partDir := filepath.Join(jobDir, part)

		// Test result.
		bytes, err := s.ReadFile(filepath.Join(partDir, "results"))
		if err != nil {
			return nil, err
		}
		var results map[string]test.Result
		if err := json.Unmarshal(bytes, &results); err != nil {
			return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
		}
		// There should be only one test in the "results" file.
		var r test.Result
		for _, curResult := range results {
			r = curResult
			break
		}
		if r.Status != test.Passed {
			data.result = false
		}

		// Failed tests.
		failedTests, err := parseFailedTests(jirix, partDir, index)
		if err != nil {
			return nil, err
		}
		data.failedTests = append(data.failedTests, failedTests...)

		// Console output.
		if aggregateOutput {
			outputBytes, err := s.ReadFile(filepath.Join(partDir, "output"))
			if err != nil {
				return nil, err
			}
			if len(partFileInfos) > 1 {
				outputs = append(outputs, fmt.Sprintf("#### Part %d ####\n%s", index, string(outputBytes)))
			} else {
				outputs = append(outputs, string(outputBytes))
			}
		}
	}
	data.output = strings.Join(outputs, "\n")
	if len(partFileInfos) > 1 {
		data.output = multiPartOutputNote + data.output
	}
	return data, nil
}

func parseFailedTests(jirix *jiri.X, jobDir string, partIndex int) ([]failedTest, error) {
	failedTests := []failedTest{}
	suitesBytes, err := jirix.NewSeq().ReadFile(filepath.Join(jobDir, "xunit.xml"))
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
					PartIndex: partIndex,
				})
			}
		}
	}
	return failedTests, nil
}

func validateValues(values url.Values) error {
	ty := values.Get("type")
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
