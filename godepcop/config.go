// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type importRuleJSON struct {
	Allow *string `json:"allow"`
	Deny  *string `json:"deny"`
	// The above fields need to be pointers to be able to distinguish
	// { "allow": "test", "deny": "" } from { "allow": "test" }
	// by looking at the return value of json.Unmarshal.
}

type packageConfigJSON struct {
	Imports []importRuleJSON `json:"imports"`
}

var configCache = map[string]*packageConfig{}

// loadPackageConfigFile loads a GO.PACKAGE configuration file located at the
// specified filesystem path.  If the call is successful, the output will be
// cached and the same instance will be returned in subsequent calls.
func loadPackageConfigFile(path string) (*packageConfig, error) {
	if p, ok := configCache[path]; ok {
		return p, nil
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := parsePackageConfig(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	p.Path = path
	configCache[path] = p
	return p, nil
}

var (
	errBothAllowDeny    = errors.New("both allow and deny are specified")
	errNeitherAllowDeny = errors.New("neither allow nor deny is specified")
	errEmptyRule        = errors.New("empty import rule")
	errNoRules          = errors.New("at least one import rule must be specified")
)

func parseImportRule(r importRuleJSON) (importRule, error) {
	switch {
	case r.Allow != nil && r.Deny != nil:
		return importRule{}, errBothAllowDeny
	case r.Allow == nil && r.Deny == nil:
		return importRule{}, errNeitherAllowDeny
	case r.Allow != nil && *r.Allow != "":
		return importRule{IsDenyRule: false, PkgExpr: *r.Allow}, nil
	case r.Deny != nil && *r.Deny != "":
		return importRule{IsDenyRule: true, PkgExpr: *r.Deny}, nil
	}
	return importRule{}, errEmptyRule
}

func parsePackageConfig(data []byte) (*packageConfig, error) {
	var pkg packageConfigJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	var rules []importRule
	for _, d := range pkg.Imports {
		rule, err := parseImportRule(d)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return nil, errNoRules
	}
	return &packageConfig{Imports: rules}, nil
}

type packageConfigIterator interface {
	Advance() bool
	Value() *packageConfig
	Err() error
}

type configFileIterator struct {
	config *packageConfig
	err    error
	depth  int
	dir    string
}

type configFileReadError struct {
	err  error
	path string
}

func (e configFileReadError) Error() string {
	return "invalid config file: " + e.path + ": " + e.err.Error()
}

const configFileName = "GO.PACKAGE"

func (c *configFileIterator) Advance() bool {
	if c.depth < 0 {
		return false
	}
	path := filepath.Join(c.dir, configFileName)
	config, err := loadPackageConfigFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			c.depth = -1
			c.err = configFileReadError{err, path}
			return false
		}
		config = &packageConfig{Path: path}
	}
	c.depth--
	c.dir = filepath.Dir(c.dir)
	c.config = config
	return true
}

func (c *configFileIterator) Value() *packageConfig {
	return c.config
}

func (c *configFileIterator) Err() error {
	return c.err
}

// newPackageConfigIterator returns an iterator over the GO.PACKAGE
// configuration files for package p.  It starts at the config file in package
// p, and then travels up successive directories until it reaches the root of
// the import path.
func newPackageConfigIterator(p *build.Package) packageConfigIterator {
	if isPseudoPackage(p) {
		return &configFileIterator{depth: -1}
	}
	return &configFileIterator{
		dir:   p.Dir,
		depth: strings.Count(p.ImportPath, "/"),
	}
}
