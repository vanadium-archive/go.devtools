package impl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"tools/lib/cmdline"
	"tools/lib/util"
)

const (
	defaultGerritBaseUrl               = "https://veyron-review.googlesource.com"
	defaultNetRcFilePath               = "/var/veyron/.netrc"
	defaultQueryString                 = "(status:open -project:experimental)"
	defaultLogFilePath                 = "/var/veyron/tmp/presubmitter_log"
	defaultPresubmitTestJenkinsProject = "veyron-presubmit-test"
	defaultTestReportPath              = "/var/veyron/tmp/test_report"
	jenkinsBaseJobUrl                  = "http://www.envyor.com/jenkins/job"
	outputPrefix                       = "[VEYRON PRESUBMIT]"
)

type credential struct {
	username string
	password string
}

var (
	gerritBaseUrlFlag               string
	netRcFilePathFlag               string
	verboseFlag                     bool
	queryStringFlag                 string
	logFilePathFlag                 string
	jenkinsHostFlag                 string
	presubmitTestJenkinsProjectFlag string
	jenkinsTokenFlag                string
	reviewMessageFlag               string
	reviewTargetRefFlag             string
	testsConfigFileFlag             string
	repoFlag                        string
	testScriptsBasePathFlag         string
	manifestFlag                    string
	jenkinsBuildNumberFlag          int
	veyronRoot                      string
	reURLUnsafeChars                *regexp.Regexp = regexp.MustCompile("[\\\\/:\\?#%]")
	reNotIdentifierChars            *regexp.Regexp = regexp.MustCompile("[^0-9A-Za-z_\\$]")
)

func init() {
	var err error
	veyronRoot, err = util.VeyronRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		return
	}

	cmdRoot.Flags.StringVar(&gerritBaseUrlFlag, "url", defaultGerritBaseUrl, "The base url of the gerrit instance.")
	cmdRoot.Flags.StringVar(&netRcFilePathFlag, "netrc", defaultNetRcFilePath, "The path to the .netrc file that stores Gerrit's credentials.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.StringVar(&jenkinsHostFlag, "host", "", "The Jenkins host. Presubmitter will not send any CLs to an empty host.")
	cmdRoot.Flags.StringVar(&jenkinsTokenFlag, "token", "", "The Jenkins API token.")
	cmdQuery.Flags.StringVar(&queryStringFlag, "query", defaultQueryString, "The string used to query Gerrit for open CLs.")
	cmdQuery.Flags.StringVar(&logFilePathFlag, "log_file", defaultLogFilePath, "The file that stores the refs from the previous Gerrit query.")
	cmdQuery.Flags.StringVar(&presubmitTestJenkinsProjectFlag, "project", defaultPresubmitTestJenkinsProject, "The name of the Jenkins project to add presubmit-test builds to.")
	cmdPost.Flags.StringVar(&reviewMessageFlag, "msg", "", "The review message to post to Gerrit.")
	cmdPost.Flags.StringVar(&reviewTargetRefFlag, "ref", "", "The ref where the review is posted.")
	cmdTest.Flags.StringVar(&testsConfigFileFlag, "conf", filepath.Join(veyronRoot, "tools", "go", "src", "tools", "presubmitter", "presubmit_tests.conf"), "The config file for presubmit tests.")
	cmdTest.Flags.StringVar(&repoFlag, "repo", "", "The URL of the repository containing the CL pointed by the ref.")
	cmdTest.Flags.StringVar(&reviewTargetRefFlag, "ref", "", "The ref where the review is posted.")
	cmdTest.Flags.StringVar(&testScriptsBasePathFlag, "tests_base_path", filepath.Join(veyronRoot, "scripts", "jenkins"), "The base path of all the test scripts.")
	cmdTest.Flags.StringVar(&manifestFlag, "manifest", "v2", "Name of the project manifest.")
	cmdTest.Flags.IntVar(&jenkinsBuildNumberFlag, "build_number", -1, "The number of the Jenkins build.")
}

// printf outputs the given message prefixed by outputPrefix.
func printf(out io.Writer, format string, args ...interface{}) {
	fmt.Fprintf(out, "%s ", outputPrefix)
	fmt.Fprintf(out, format, args...)
}

// Root returns a command that represents the root of the presubmitter tool.
func Root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the presubmitter tool.
var cmdRoot = &cmdline.Command{
	Name:     "presubmitter",
	Short:    "Tool for performing various presubmit related functions",
	Long:     "The presubmitter tool performs various presubmit related functions.",
	Children: []*cmdline.Command{cmdQuery, cmdPost, cmdTest, cmdVersion},
}

// cmdVersion represent the 'version' command of the presubmitter tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the presubmitter tool.",
}

// Version should be over-written during build:
//
// go build -ldflags "-X tools/presubmitter/impl.Version <version>" tools/presubmitter
var Version string = "manual-build"

func runVersion(command *cmdline.Command, _ []string) error {
	printf(command.Stdout(), "presubmitter tool version %v\n", Version)
	return nil
}
