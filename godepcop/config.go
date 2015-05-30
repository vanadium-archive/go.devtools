// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	XMLName    struct{} `xml:"godepcop"`
	PkgRules   []rule   `xml:"pkg"`
	TestRules  []rule   `xml:"test"`
	XTestRules []rule   `xml:"xtest"`
	Path       string   `xml:"-"`
}

type rule struct {
	// The fields are pointers so that we can distinguish empty from unset values.
	Allow *string `xml:"allow,attr,omitempty"`
	Deny  *string `xml:"deny,attr,omitempty"`
}

func (r rule) IsDeny() bool {
	return r.Deny != nil
}

func (r rule) Pattern() string {
	switch {
	case r.Allow != nil:
		return *r.Allow
	case r.Deny != nil:
		return *r.Deny
	}
	return ""
}

func (r rule) Validate() error {
	switch {
	case r.Allow == nil && r.Deny == nil:
		return errNeitherAllowDeny
	case r.Allow != nil && r.Deny != nil:
		return errBothAllowDeny
	case r.Allow != nil && *r.Allow != "":
		return nil
	case r.Deny != nil && *r.Deny != "":
		return nil
	}
	return errEmptyRule
}

var configCache = map[string]*config{}

// loadConfig loads a .godepcop configuration file located at the specified
// filesystem path.  If the call is successful, the output will be cached and
// the same instance will be returned in subsequent calls.
func loadConfig(path string) (*config, error) {
	if p, ok := configCache[path]; ok {
		return p, nil
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := parseConfig(data)
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
	errEmptyRule        = errors.New("empty rule")
	errNoRules          = errors.New("at least one rule must be specified")
)

func parseConfig(data []byte) (*config, error) {
	c := new(config)
	if err := xml.Unmarshal(data, c); err != nil {
		return nil, err
	}
	if len(c.PkgRules) == 0 && len(c.TestRules) == 0 && len(c.XTestRules) == 0 {
		return nil, errNoRules
	}
	for _, r := range c.PkgRules {
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("pkg: %v", err)
		}
	}
	for _, r := range c.TestRules {
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("test: %v", err)
		}
	}
	for _, r := range c.XTestRules {
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("xtest: %v", err)
		}
	}
	return c, nil
}

type configIter struct {
	cfg   *config
	err   error
	depth int
	dir   string
}

const configFileName = ".godepcop"

func (c *configIter) Advance() bool {
	if c.depth < 0 {
		return false
	}
	path := filepath.Join(c.dir, configFileName)
	cfg, err := loadConfig(path)
	if err != nil {
		if !os.IsNotExist(err) {
			c.depth = -1
			c.err = err
			return false
		}
		cfg = &config{Path: path}
	}
	c.depth--
	c.dir = filepath.Dir(c.dir)
	c.cfg = cfg
	return true
}

func (c *configIter) Value() *config { return c.cfg }
func (c *configIter) Err() error     { return c.err }

// newConfigIter returns an iterator over the .godepcop configuration files for
// package p.  It starts at the config file in package p, and then travels up
// successive directories until it reaches the root of the import path.
func newConfigIter(p *build.Package) *configIter {
	if isPseudoPackage(p) {
		return &configIter{depth: -1}
	}
	return &configIter{
		dir:   p.Dir,
		depth: strings.Count(p.ImportPath, "/"),
	}
}
