// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

const (
	defaultGerritBaseUrl    = "https://vanadium-review.googlesource.com"
	defaultNetRcFilePath    = "${HOME}/.netrc"
	defaultPresubmitTestJob = "vanadium-presubmit-test"
	defaultQueryString      = "(status:open -project:experimental)"
	jenkinsBaseJobUrl       = "https://veyron.corp.google.com/jenkins/job"
	outputPrefix            = "[VANADIUM PRESUBMIT]"
)

type credential struct {
	username string
	password string
}

var (
	dryRunFlag             bool
	gerritBaseUrlFlag      string
	jenkinsHostFlag        string
	jenkinsBuildNumberFlag int
	manifestFlag           string
	netRcFilePathFlag      flag.Getter // TODO(jsimsa): Move this flag to query.go.
	colorFlag              bool
	presubmitTestJobFlag   string
	verboseFlag            bool
)

func init() {
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	cmdRoot.Flags.StringVar(&gerritBaseUrlFlag, "url", defaultGerritBaseUrl, "The base url of the gerrit instance.")
	cmdRoot.Flags.StringVar(&jenkinsHostFlag, "host", "", "The Jenkins host. Presubmit will not send any CLs to an empty host.")
	netRcFilePathFlag = cmdline.EnvFlag(defaultNetRcFilePath)
	cmdRoot.Flags.Var(netRcFilePathFlag, "netrc", "The path to the .netrc file that stores Gerrit's credentials.")
	cmdRoot.Flags.BoolVar(&colorFlag, "color", true, "Use color to format output.")
	cmdRoot.Flags.StringVar(&presubmitTestJobFlag, "job", defaultPresubmitTestJob, "The name of the Jenkins job to add presubmit-test builds to.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
}

var (
	reURLUnsafeChars     *regexp.Regexp = regexp.MustCompile("[\\\\/:\\?#%]")
	reNotIdentifierChars *regexp.Regexp = regexp.MustCompile("[^0-9A-Za-z_\\$]")
	vroot                string
)

// printf outputs the given message prefixed by outputPrefix, adding a
// blank line before any messages that start with "###".
func printf(out io.Writer, format string, args ...interface{}) {
	if strings.HasPrefix(format, "###") {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s ", outputPrefix)
	fmt.Fprintf(out, format, args...)
}

// root returns a command that represents the root of the presubmit tool.
func root() *cmdline.Command {
	var err error
	vroot, err = util.VanadiumRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}
	return cmdRoot
}

// cmdRoot represents the root of the presubmit tool.
var cmdRoot = &cmdline.Command{
	Name:     "presubmit",
	Short:    "Tool for performing various presubmit related functions",
	Long:     "The presubmit tool performs various presubmit related functions.",
	Children: []*cmdline.Command{cmdQuery, cmdResult, cmdTest, cmdVersion},
}

// cmdVersion represent the 'version' command of the presubmit tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the presubmit tool.",
}

func runVersion(command *cmdline.Command, _ []string) error {
	printf(command.Stdout(), "presubmit tool version %v\n", tool.Version)
	return nil
}
