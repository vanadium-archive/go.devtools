// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var (
	abc, xyz, dots = "abc", "xyz", "..."

	testConfigXML = `
<godepcop>
  <pkg allow="abc"/>
  <pkg allow="xyz"/>
  <pkg deny="..."/>
  <test allow="..."/>
  <xtest deny="..."/>
</godepcop>
`

	testConfig = &config{
		PkgRules:   []rule{{Allow: &abc}, {Allow: &xyz}, {Deny: &dots}},
		TestRules:  []rule{{Allow: &dots}},
		XTestRules: []rule{{Deny: &dots}},
	}
)

func TestLoadConfig(t *testing.T) {
	// Create and load a config file.
	dir, err := ioutil.TempDir("", "godepcop")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, ".godepcop")
	if err := ioutil.WriteFile(path, []byte(testConfigXML), os.ModePerm); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", path, err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Errorf("loadConfig failed: %v", err)
	}
	// Compare the loaded config against our expectations.
	cpConfig := *testConfig
	cpConfig.Path = path
	if got, want := cfg, &cpConfig; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Make sure non-existent files return an error.
	cfg, err = loadConfig(path + ".XYZ")
	if cfg != nil || err == nil || !os.IsNotExist(err) {
		t.Errorf("got (%v, %v), want (nil, NoExist)", cfg, err)
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		Data   string
		Config *config
	}{
		{
			`<godepcop><pkg allow="..."/></godepcop>`,
			&config{PkgRules: []rule{{Allow: &dots}}},
		},
		{
			`<godepcop><pkg deny="..."/></godepcop>`,
			&config{PkgRules: []rule{{Deny: &dots}}},
		},
		{
			`<godepcop><pkg allow="abc"/><pkg deny="..."/></godepcop>`,
			&config{PkgRules: []rule{{Allow: &abc}, {Deny: &dots}}},
		},
		{
			testConfigXML,
			testConfig,
		},
	}
	for _, test := range tests {
		cfg, err := parseConfig([]byte(test.Data))
		if err != nil {
			t.Errorf("%s failed: %v", test.Data, err)
		}
		if got, want := cfg, test.Config; !reflect.DeepEqual(got, want) {
			t.Errorf("%s got %v, want %v", test.Data, got, want)
		}
	}
}

func TestParseConfigError(t *testing.T) {
	tests := []struct {
		Data string
		Err  string
	}{
		// XML syntax errors
		{``, "*"},
		{`<godepcop>`, "*"},
		// No rules
		{
			`<godepcop/>`,
			"at least one rule must be specified",
		},
		{
			`<godepcop></godepcop>`,
			"at least one rule must be specified",
		},
		// Pkg rules
		{
			`<godepcop><pkg/></godepcop>`,
			"pkg: neither allow nor deny is specified",
		},
		{
			`<godepcop><pkg foo=""/></godepcop>`,
			"pkg: neither allow nor deny is specified",
		},
		{
			`<godepcop><pkg allow=""/></godepcop>`,
			"pkg: empty rule",
		},
		{
			`<godepcop><pkg deny=""/></godepcop>`,
			"pkg: empty rule",
		},
		{
			`<godepcop><pkg allow="x" deny="y"/></godepcop>`,
			"pkg: both allow and deny are specified",
		},
		// Test rules
		{
			`<godepcop><test/></godepcop>`,
			"test: neither allow nor deny is specified",
		},
		{
			`<godepcop><test foo=""/></godepcop>`,
			"test: neither allow nor deny is specified",
		},
		{
			`<godepcop><test allow=""/></godepcop>`,
			"test: empty rule",
		},
		{
			`<godepcop><test deny=""/></godepcop>`,
			"test: empty rule",
		},
		{
			`<godepcop><test allow="x" deny="y"/></godepcop>`,
			"test: both allow and deny are specified",
		},
		// XTest rules
		{
			`<godepcop><xtest/></godepcop>`,
			"xtest: neither allow nor deny is specified",
		},
		{
			`<godepcop><xtest foo=""/></godepcop>`,
			"xtest: neither allow nor deny is specified",
		},
		{
			`<godepcop><xtest allow=""/></godepcop>`,
			"xtest: empty rule",
		},
		{
			`<godepcop><xtest deny=""/></godepcop>`,
			"xtest: empty rule",
		},
		{
			`<godepcop><xtest allow="x" deny="y"/></godepcop>`,
			"xtest: both allow and deny are specified",
		},
	}
	for _, test := range tests {
		cfg, err := parseConfig([]byte(test.Data))
		if cfg != nil {
			t.Errorf("%s got %v, want nil", test.Data, cfg)
		}
		if test.Err == "*" {
			if err == nil {
				t.Errorf("%s got error nil, want %v", test.Data, test.Err)
			}
		} else {
			if got, want := err.Error(), test.Err; got != want {
				t.Errorf("%s got error %v, want %v", test.Data, got, want)
			}
		}
	}
}
