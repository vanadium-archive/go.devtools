package testutil

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/runutil"
	"v.io/tools/lib/util"
)

// generateTestSuite generates an xUnit test suite that encapsulates
// the given input.
func generateTestSuite(ctx *util.Context, success bool, pkg string, duration time.Duration, output string) *testSuite {
	// Generate an xUnit test suite describing the result.
	s := testSuite{Name: pkg}
	c := testCase{
		Classname: pkg,
		Name:      "Test",
		Time:      fmt.Sprintf("%.2f", duration.Seconds()),
	}
	if !success {
		fmt.Fprintf(ctx.Stdout(), "%s ... failed\n%v\n", pkg, output)
		f := testFailure{
			Message: "vrpc",
			Data:    output,
		}
		c.Failures = append(c.Failures, f)
		s.Failures++
	} else {
		fmt.Fprintf(ctx.Stdout(), "%s ... ok\n", pkg)
	}
	s.Tests++
	s.Cases = append(s.Cases, c)
	return &s
}

// testProdService test the given production service.
func testProdService(ctx *util.Context, service prodService) (*testSuite, error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(root, "release", "go", "bin", "vrpc")
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	start := time.Now()
	if err := ctx.Run().TimedCommandWithOpts(DefaultTestTimeout, opts, bin, "signature", service.objectName); err != nil {
		return generateTestSuite(ctx, false, service.name, time.Now().Sub(start), out.String()), nil
	}
	if !service.regexp.Match(out.Bytes()) {
		fmt.Fprintf(ctx.Stderr(), "couldn't match regexp `%s` in output:\n%v\n", service.regexp, out.String())
		return generateTestSuite(ctx, false, service.name, time.Now().Sub(start), "mismatching signature"), nil
	}
	return generateTestSuite(ctx, true, service.name, time.Now().Sub(start), ""), nil
}

type prodService struct {
	name       string
	objectName string
	regexp     *regexp.Regexp
}

// vanadiumProdServicesTest runs a test of vanadium production services.
func vanadiumProdServicesTest(ctx *util.Context, testName string) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, result, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, err
	} else if result != nil {
		return result, nil
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install the vrpc tool.
	if testResult, err := genXUnitReportOnCmdError(ctx, testName, "BuildTools", "VRPC build failure",
		func(opts runutil.Opts) error {
			return ctx.Run().CommandWithOpts(opts, "v23", "go", "install", "v.io/core/veyron/tools/vrpc")
		}); err != nil {
		return nil, err
	} else if testResult != nil {
		return testResult, nil
	}

	// Describe the test cases.
	namespaceRoot := "/ns.dev.v.io:8101"
	allPassed, suites := true, []testSuite{}
	services := []prodService{
		prodService{
			name:       "mounttable",
			objectName: namespaceRoot,
			regexp:     regexp.MustCompile(`MountTable[[:space:]]+interface`),
		},
		prodService{
			name:       "application repository",
			objectName: namespaceRoot + "/applicationd",
			regexp:     regexp.MustCompile(`Application[[:space:]]+interface`),
		},
		prodService{
			name:       "binary repository",
			objectName: namespaceRoot + "/binaryd",
			regexp:     regexp.MustCompile(`Binary[[:space:]]+interface`),
		},
		prodService{
			name:       "macaroon service",
			objectName: namespaceRoot + "/identity/dev.v.io/macaroon",
			regexp:     regexp.MustCompile(`MacaroonBlesser[[:space:]]+interface`),
		},
		prodService{
			name:       "google identity service",
			objectName: namespaceRoot + "/identity/dev.v.io/google",
			regexp:     regexp.MustCompile(`OAuthBlesser[[:space:]]+interface`),
		},
		prodService{
			name:       "binary discharger",
			objectName: namespaceRoot + "/identity/dev.v.io/discharger",
			regexp:     regexp.MustCompile(`Discharger[[:space:]]+interface`),
		},
	}

	for _, service := range services {
		suite, err := testProdService(ctx, service)
		if err != nil {
			return nil, err
		}
		allPassed = allPassed && (suite.Failures == 0)
		suites = append(suites, *suite)
	}

	// Create the xUnit report.
	if err := createXUnitReport(ctx, testName, suites); err != nil {
		return nil, err
	}
	if !allPassed {
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
