package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"v.io/x/devtools/lib/util"
	"v.io/x/devtools/lib/version"
	"v.io/x/lib/cmdline"
)

const (
	defaultConfigFile       = "$VANADIUM_ROOT/release/go/src/v.io/x/devtools/conf/presubmit"
	defaultGerritBaseUrl    = "https://vanadium-review.googlesource.com"
	defaultLogFilePath      = "$HOME/tmp/presubmit_log"
	defaultNetRcFilePath    = "$HOME/.netrc"
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
	// flags
	dryRunFlag             bool
	gerritBaseUrlFlag      string
	jenkinsBuildNumberFlag int
	jenkinsHostFlag        string
	logFilePathFlag        string
	manifestFlag           string
	netRcFilePathFlag      string
	noColorFlag            bool
	presubmitTestJobFlag   string
	queryStringFlag        string
	projectsFlag           string
	reviewMessageFlag      string
	reviewTargetRefsFlag   string
	testFlag               string
	verboseFlag            bool

	reURLUnsafeChars     *regexp.Regexp = regexp.MustCompile("[\\\\/:\\?#%]")
	reNotIdentifierChars *regexp.Regexp = regexp.MustCompile("[^0-9A-Za-z_\\$]")
	vroot                string
)

func init() {
	cmdRoot.Flags.StringVar(&gerritBaseUrlFlag, "url", defaultGerritBaseUrl, "The base url of the gerrit instance.")
	cmdRoot.Flags.StringVar(&netRcFilePathFlag, "netrc", defaultNetRcFilePath, "The path to the .netrc file that stores Gerrit's credentials.")
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.StringVar(&jenkinsHostFlag, "host", "", "The Jenkins host. Presubmit will not send any CLs to an empty host.")
	cmdRoot.Flags.StringVar(&presubmitTestJobFlag, "job", defaultPresubmitTestJob, "The name of the Jenkins job to add presubmit-test builds to.")
	cmdRoot.Flags.BoolVar(&noColorFlag, "nocolor", false, "Do not use color to format output.")
	cmdQuery.Flags.StringVar(&queryStringFlag, "query", defaultQueryString, "The string used to query Gerrit for open CLs.")
	cmdQuery.Flags.StringVar(&logFilePathFlag, "log_file", defaultLogFilePath, "The file that stores the refs from the previous Gerrit query.")
	cmdQuery.Flags.StringVar(&manifestFlag, "manifest", "default", "Name of the project manifest.")
	cmdResult.Flags.StringVar(&projectsFlag, "projects", "", "The base names of the remote projects containing the CLs pointed by the refs, separated by ':'.")
	cmdResult.Flags.StringVar(&reviewTargetRefsFlag, "refs", "", "The review references separated by ':'.")
	cmdResult.Flags.IntVar(&jenkinsBuildNumberFlag, "build_number", -1, "The number of the Jenkins build.")
	cmdTest.Flags.StringVar(&projectsFlag, "projects", "", "The base names of the remote projects containing the CLs pointed by the refs, separated by ':'.")
	cmdTest.Flags.StringVar(&reviewTargetRefsFlag, "refs", "", "The review references separated by ':'.")
	cmdTest.Flags.StringVar(&manifestFlag, "manifest", "default", "Name of the project manifest.")
	cmdTest.Flags.IntVar(&jenkinsBuildNumberFlag, "build_number", -1, "The number of the Jenkins build.")
	cmdTest.Flags.StringVar(&testFlag, "test", "", "The name of a single test to run.")
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

// substituteVarsInFlags substitutes environment variables in default
// values of relevant flags.
func substituteVarsInFlags() {
	var err error
	vroot, err = util.VanadiumRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}
	if logFilePathFlag == defaultLogFilePath {
		logFilePathFlag = filepath.Join(os.Getenv("HOME"), "tmp", "presubmit_log")
	}
	if netRcFilePathFlag == defaultNetRcFilePath {
		netRcFilePathFlag = filepath.Join(os.Getenv("HOME"), ".netrc")
	}
}

// root returns a command that represents the root of the presubmit tool.
func root() *cmdline.Command {
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
	printf(command.Stdout(), "presubmit tool version %v\n", version.Version)
	return nil
}
