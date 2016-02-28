// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/retry"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

// generateXUnitTestSuite generates an xUnit test suite that
// encapsulates the given input.
func generateXUnitTestSuite(jirix *jiri.X, failure *xunit.Failure, pkg string, duration time.Duration) *xunit.TestSuite {
	// Generate an xUnit test suite describing the result.
	s := xunit.TestSuite{Name: pkg}
	c := xunit.TestCase{
		Classname: pkg,
		Name:      "Test",
		Time:      fmt.Sprintf("%.2f", duration.Seconds()),
	}
	if failure != nil {
		fmt.Fprintf(jirix.Stdout(), "%s ... failed\n%v\n", pkg, failure.Data)
		c.Failures = append(c.Failures, *failure)
		s.Failures++
	} else {
		fmt.Fprintf(jirix.Stdout(), "%s ... ok\n", pkg)
	}
	s.Tests++
	s.Cases = append(s.Cases, c)
	return &s
}

// testSingleProdService test the given production service.
func testSingleProdService(jirix *jiri.X, principalDir string, service prodService) *xunit.TestSuite {
	bin := filepath.Join(jirix.Root, "release", "go", "bin", "vrpc")
	var out bytes.Buffer
	start := time.Now()
	args := []string{}
	if principalDir != "" {
		args = append(args, "--v23.credentials", principalDir)
	}
	args = append(args, "signature", "-s", "--show-reserved")
	if principalDir == "" {
		args = append(args, "--insecure")
	}
	args = append(args, service.objectName)
	if err := jirix.NewSeq().Capture(&out, &out).Verbose(true).Timeout(test.DefaultTimeout).
		Last(bin, args...); err != nil {
		fmt.Fprintf(jirix.Stderr(), "Failed running %q: %v. Output:\n%v\n", append([]string{bin}, args...), err, out.String())
		return generateXUnitTestSuite(jirix, &xunit.Failure{Message: "vrpc", Data: out.String()}, service.name, time.Now().Sub(start))
	}
	if !service.regexp.Match(out.Bytes()) {
		fmt.Fprintf(jirix.Stderr(), "couldn't match regexp %q in output:\n%v\n", service.regexp, out.String())
		return generateXUnitTestSuite(jirix, &xunit.Failure{Message: "vrpc", Data: "mismatching signature"}, service.name, time.Now().Sub(start))
	}
	return generateXUnitTestSuite(jirix, nil, service.name, time.Now().Sub(start))
}

type prodService struct {
	name       string         // Name to use for the test description
	objectName string         // Object name of the service to connect to
	regexp     *regexp.Regexp // Regexp that should match the signature output
}

// vanadiumProdServicesTest runs a test of vanadium production services.
func vanadiumProdServicesTest(jirix *jiri.X, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	// Need the new-stype base profile since many web tests will build
	// go apps that need it.
	cleanup, err := initTest(jirix, testName, []string{"base"})
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install the vrpc tool.
	tmpdir, err := jirix.NewSeq().Run("jiri", "go", "install", "v.io/x/ref/cmd/vrpc").
		Run("jiri", "go", "install", "v.io/x/ref/cmd/principal").
		TempDir("", "prod-services-test")
	if err != nil {
		return nil, newInternalError(err, "Installing vrpc and creating testdir")
	}
	defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpdir).Done() }, &e)

	blessingRoot, namespaceRoot := getServiceOpts(opts)
	allPassed, suites := true, []xunit.TestSuite{}

	// Fetch the "root" blessing that all services are blessed by.
	suite, pubkey, blessingNames := testIdentityProviderHTTP(jirix, blessingRoot)
	suites = append(suites, *suite)

	if suite.Failures == 0 {
		// Setup a principal that will be used by testAllProdServices and will
		// recognize the blessings of the prod services.
		principalDir, err := setupPrincipal(jirix, tmpdir, pubkey, blessingNames)
		if err != nil {
			return nil, err
		}
		for _, suite := range testAllProdServices(jirix, principalDir, namespaceRoot) {
			allPassed = allPassed && (suite.Failures == 0)
			suites = append(suites, *suite)
		}
	}

	// Create the xUnit report.
	if err := xunit.CreateReport(jirix, testName, suites); err != nil {
		return nil, err
	}
	for _, suite := range suites {
		if suite.Failures > 0 {
			// At least one test failed:
			return &test.Result{Status: test.Failed}, nil
		}
	}
	return &test.Result{Status: test.Passed}, nil
}

func testAllProdServices(jirix *jiri.X, principalDir, namespaceRoot string) []*xunit.TestSuite {
	services := []prodService{
		prodService{
			name:       "mounttable",
			objectName: namespaceRoot,
			regexp:     regexp.MustCompile(`MountTable[[:space:]]+interface`),
		},
		prodService{
			name:       "application repository",
			objectName: namespaceRoot + "/applications",
			regexp:     regexp.MustCompile(`Application[[:space:]]+interface`),
		},
		prodService{
			name:       "binary repository",
			objectName: namespaceRoot + "/binaries",
			regexp:     regexp.MustCompile(`Binary[[:space:]]+interface`),
		},
		prodService{
			name:       "macaroon service",
			objectName: namespaceRoot + "/identity/dev.v.io:u/macaroon",
			regexp:     regexp.MustCompile(`MacaroonBlesser[[:space:]]+interface`),
		},
		prodService{
			name:       "google identity service",
			objectName: namespaceRoot + "/identity/dev.v.io:u/google",
			regexp:     regexp.MustCompile(`OAuthBlesser[[:space:]]+interface`),
		},
		prodService{
			objectName: namespaceRoot + "/identity/dev.v.io:u/discharger",
			name:       "binary discharger",
			regexp:     regexp.MustCompile(`Discharger[[:space:]]+interface`),
		},
		prodService{
			objectName: namespaceRoot + "/proxy-mon/__debug",
			name:       "proxy service",
			// We just check that the returned signature has the __Reserved interface since
			// proxy-mon doesn't implement any other services.
			regexp: regexp.MustCompile(`__Reserved[[:space:]]+interface`),
		},
	}

	var suites []*xunit.TestSuite
	for _, service := range services {
		suites = append(suites, testSingleProdService(jirix, principalDir, service))
	}
	return suites
}

// testIdentityProviderHTTP tests that the identity provider's HTTP server is
// up and running and also fetches the set of blessing names that the provider
// claims to be authoritative on and the public key (encoded) used by that
// identity provider to sign certificates for blessings.
//
// PARANOIA ALERT:
// This function is subject to man-in-the-middle attacks because it does not
// verify the TLS certificates presented by the server. This does open the
// door for an attack where a parallel universe of services could be setup
// and fool this production services test into thinking all services are
// up and running when they may not be.
//
// The attacker in this case will have to be able to mess with the routing
// tables on the machine running this test, or the network routes of routers
// used by the machine, or mess up DNS entries.
func testIdentityProviderHTTP(jirix *jiri.X, blessingRoot string) (suite *xunit.TestSuite, publickey string, blessingNames []string) {
	url := fmt.Sprintf("https://%s/auth/blessing-root", blessingRoot)
	var response struct {
		Names     []string `json:"names"`
		PublicKey string   `json:"publicKey"`
	}
	var resp *http.Response
	var err error
	var start time.Time
	fn := func() error {
		start = time.Now()
		resp, err = http.Get(url)
		return err
	}
	if err = retry.Function(jirix.Context, fn); err == nil {
		defer resp.Body.Close()
		err = json.NewDecoder(resp.Body).Decode(&response)
	}
	var failure *xunit.Failure
	if err != nil {
		failure = &xunit.Failure{Message: "identityd HTTP", Data: err.Error()}
	}
	return generateXUnitTestSuite(jirix, failure, url, time.Now().Sub(start)), response.PublicKey, response.Names
}

func setupPrincipal(jirix *jiri.X, tmpdir, pubkey string, blessingNames []string) (string, error) {
	s := jirix.NewSeq()
	dir := filepath.Join(tmpdir, "credentials")
	bin := filepath.Join(jirix.Root, "release", "go", "bin", "principal")
	if err := s.Timeout(test.DefaultTimeout).Last(bin, "create", dir, "prod-services-tester"); err != nil {
		fmt.Fprintf(jirix.Stderr(), "principal create failed: %v\n", err)
		return "", err
	}
	for _, name := range blessingNames {
		if err := s.Timeout(test.DefaultTimeout).Last(bin, "--v23.credentials", dir, "recognize", name, pubkey); err != nil {
			fmt.Fprintf(jirix.Stderr(), "principal recognize %v %v failed: %v\n", name, pubkey, err)
			return "", err
		}
	}
	return dir, nil
}

// getServiceOpts extracts blessing root and namespace root from the
// given Opts.
func getServiceOpts(opts []Opt) (string, string) {
	blessingRoot := "dev.v.io"
	namespaceRoot := "/ns.dev.v.io:8101"
	for _, opt := range opts {
		switch v := opt.(type) {
		case BlessingsRootOpt:
			blessingRoot = string(v)
		case NamespaceRootOpt:
			namespaceRoot = string(v)
		}
	}
	return blessingRoot, namespaceRoot
}
