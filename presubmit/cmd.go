// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"v.io/x/devtools/internal/project"
	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

const (
	defaultGerritBaseUrl    = "https://vanadium-review.googlesource.com"
	defaultPresubmitTestJob = "vanadium-presubmit-test"
	defaultQueryString      = "(status:open -project:experimental)"
	jenkinsBaseJobUrl       = "https://veyron.corp.google.com/jenkins/job"
	outputPrefix            = "[VANADIUM PRESUBMIT]"
)

var (
	gerritBaseUrlFlag      string
	jenkinsHostFlag        string
	jenkinsBuildNumberFlag int
	presubmitTestJobFlag   string
)

func init() {
	cmdRoot.Flags.StringVar(&gerritBaseUrlFlag, "url", defaultGerritBaseUrl, "The base url of the gerrit instance.")
	cmdRoot.Flags.StringVar(&jenkinsHostFlag, "host", "", "The Jenkins host. Presubmit will not send any CLs to an empty host.")
	cmdRoot.Flags.StringVar(&presubmitTestJobFlag, "job", defaultPresubmitTestJob, "The name of the Jenkins job to add presubmit-test builds to.")

	tool.InitializeRunFlags(&cmdRoot.Flags)
}

var (
	reURLUnsafeChars     *regexp.Regexp = regexp.MustCompile("[\\\\/:\\?#%]")
	reNotIdentifierChars *regexp.Regexp = regexp.MustCompile("[^0-9A-Za-z_\\$]")
	vroot                string
)

func main() {
	var err error
	vroot, err = project.V23Root()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	cmdline.Main(cmdRoot)
}

// printf outputs the given message prefixed by outputPrefix, adding a
// blank line before any messages that start with "###".
func printf(out io.Writer, format string, args ...interface{}) {
	if strings.HasPrefix(format, "###") {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s ", outputPrefix)
	fmt.Fprintf(out, format, args...)
}

// cmdRoot represents the root of the presubmit tool.
var cmdRoot = &cmdline.Command{
	Name:  "presubmit",
	Short: "Perform Vanadium presubmit related functions",
	Long: `
Command presubmit performs Vanadium presubmit related functions.
`,
	Children: []*cmdline.Command{cmdQuery, cmdResult, cmdTest, cmdVersion},
}

// cmdVersion represent the 'version' command of the presubmit tool.
var cmdVersion = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runVersion),
	Name:   "version",
	Short:  "Print version",
	Long:   "Print version of the presubmit tool.",
}

func runVersion(env *cmdline.Env, _ []string) error {
	printf(env.Stdout, "presubmit tool version %v\n", tool.Version)
	return nil
}
