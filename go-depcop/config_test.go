// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadPackageConfigFile(t *testing.T) {
	tests := []packageConfig{
		{"testdata/load-test.depcop", []importRule{
			{IsDenyRule: true, PkgExpr: "denied-package/x"},
			{IsDenyRule: false, PkgExpr: "allowed-package/y"},
		}},
		{"testdata/nacl-app.depcop", []importRule{
			{IsDenyRule: true, PkgExpr: "syscall"},
		}},
		{"testdata/v23-rt.depcop", []importRule{
			{IsDenyRule: false, PkgExpr: "v23/runtimes/..."},
		}},
		{"testdata/v23.depcop", []importRule{
			{IsDenyRule: false, PkgExpr: "v23/..."},
			{IsDenyRule: true, PkgExpr: "..."},
		}},
	}
	for _, test := range tests {
		config, err := loadPackageConfigFile(test.Path)
		if err != nil {
			t.Fatal("error reading config file:", err)
		}
		if got, want := config.Imports, test.Imports; !reflect.DeepEqual(got, want) {
			t.Errorf("%s got %v, want %v", test.Path, got, want)
		}
	}
}

func TestLoadPackageConfigFileNotExist(t *testing.T) {
	config, err := loadPackageConfigFile("testdata/non-existent.depcop")
	if config != nil || err == nil || !os.IsNotExist(err) {
		t.Errorf("got (%v, %v), want (nil, NoExist)", config, err)
	}
}

func TestParsePackageConfig(t *testing.T) {
	tests := []struct {
		Data   string
		Config *packageConfig
	}{
		{
			`{"imports": [{"allow": "..."}]}`,
			&packageConfig{Imports: []importRule{
				{IsDenyRule: false, PkgExpr: "..."},
			}},
		},
		{
			`{"imports": [{"deny": "..."}]}`,
			&packageConfig{Imports: []importRule{
				{IsDenyRule: true, PkgExpr: "..."},
			}},
		},
		{
			`{"imports": [{"allow": "..."}, {"deny": "..."}]}`,
			&packageConfig{Imports: []importRule{
				{IsDenyRule: false, PkgExpr: "..."},
				{IsDenyRule: true, PkgExpr: "..."},
			}},
		},
		{
			`{"imports": [{"allow": "foo"}, {"allow": "bar"}, {"deny": "baz/..."}]}`,
			&packageConfig{Imports: []importRule{
				{IsDenyRule: false, PkgExpr: "foo"},
				{IsDenyRule: false, PkgExpr: "bar"},
				{IsDenyRule: true, PkgExpr: "baz/..."},
			}},
		},
	}
	for _, test := range tests {
		config, err := parsePackageConfig([]byte(test.Data))
		if err != nil {
			t.Errorf("%s failed: %v", test.Data, err)
		}
		if got, want := test.Config, config; !reflect.DeepEqual(got, want) {
			t.Errorf("%s got %v, want %v", test.Data, got, want)
		}
	}
}

func TestParsePackageConfigError(t *testing.T) {
	tests := []string{
		``,
		`{}`,
		`[]`,
		`{"foo": ""}`,
		`{"foo": []}`,
		`{"foo": [{}]}`,
		`{"foo": [{"allow": "v23/rt/..."}]}`,
		`{"imports": ""}`,
		`{"imports": []}`,
		`{"imports": ["foo"]}`,
		`{"imports": [{}]}`,
		`{"imports": [{"foo": "v23/rt/..."}]}`,
		`{"imports": [{"allow": "v23/rt/...", "deny": "bar"}]}`,
		`{"imports": [{"allow": ""}]}`,
		`{"imports": [{"deny": ""}]}`,
	}
	for _, test := range tests {
		config, err := parsePackageConfig([]byte(test))
		if config != nil || err == nil {
			t.Errorf("%s got (%v, %v), want (nil, error)", test, config, err)
		}
	}
}
