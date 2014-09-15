package impl

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/gerrit"
	"tools/lib/util"
)

const (
	defaultGerritBaseUrl               = "https://veyron-review.googlesource.com"
	defaultNetRcFilePath               = "/var/veyron/.netrc"
	defaultQueryString                 = "(status:open -label:Code-Review=2 -project:experimental)"
	defaultLogFilePath                 = "/var/veyron/tmp/presubmitter_log"
	defaultPresubmitTestJenkinsProject = "veyron-presubmit-test"
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
	manifestFlag                    string
)

func init() {
	cmdRoot.Flags.StringVar(&gerritBaseUrlFlag, "url", defaultGerritBaseUrl, "The base url of the gerrit instance.")
	cmdRoot.Flags.StringVar(&netRcFilePathFlag, "netrc", defaultNetRcFilePath, "The path to the .netrc file that stores Gerrit's credentials.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdQuery.Flags.StringVar(&queryStringFlag, "query", defaultQueryString, "The string used to query Gerrit for open CLs.")
	cmdQuery.Flags.StringVar(&logFilePathFlag, "log_file", defaultLogFilePath, "The file that stores the refs from the previous Gerrit query.")
	cmdQuery.Flags.StringVar(&jenkinsHostFlag, "host", "", "The Jenkins host. Presubmitter will not send any CLs to an empty host.")
	cmdQuery.Flags.StringVar(&presubmitTestJenkinsProjectFlag, "project", defaultPresubmitTestJenkinsProject, "The name of the Jenkins project to add presubmit-test builds to.")
	cmdQuery.Flags.StringVar(&jenkinsTokenFlag, "token", "", "The Jenkins API token.")
	cmdPost.Flags.StringVar(&reviewMessageFlag, "msg", "", "The review message to post to Gerrit.")
	cmdPost.Flags.StringVar(&reviewTargetRefFlag, "ref", "", "The ref where the review is posted.")
	cmdSelfUpdate.Flags.StringVar(&manifestFlag, "manifest", "absolute", "Name of the project manifest.")
}

// Root returns a command that represents the root of the presubmitter tool.
func Root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the presubmitter tool.
var cmdRoot = &cmdline.Command{
	Name:     "presubmitter",
	Short:    "Command-line tool for various presubmit related functionalities",
	Long:     "Command-line tool for various presubmit related functionalities.",
	Children: []*cmdline.Command{cmdQuery, cmdPost, cmdSelfUpdate, cmdVersion},
}

// cmdQuery represents the 'query' command of the presubmitter tool.
var cmdQuery = &cmdline.Command{
	Name:  "query",
	Short: "Query open CLs from Gerrit",
	Long: `
This subcommand queries open CLs from Gerrit, calculates diffs from the previous
query results, and sends each one with related metadata (ref, repo, changeId) to
a Jenkins project which will run tests against the corresponding CL and post review
with test results.
`,
	Run: runQuery,
}

// runQuery implements the "query" subcommand.
func runQuery(*cmdline.Command, []string) error {
	// Basic sanity check for the Gerrit base url.
	gerritHost, err := checkGerritBaseUrl()
	if err != nil {
		return err
	}

	// Parse .netrc file to get Gerrit credential.
	gerritCred, err := gerritHostCredential(gerritHost)
	if err != nil {
		return err
	}

	// Read refs from the log file.
	prevRefs, err := readLog()
	if err != nil {
		return err
	}

	// Query Gerrit.
	username, password := gerritCred.username, gerritCred.password
	curQueryResults, err := gerrit.Query(gerritBaseUrlFlag, username, password, queryStringFlag)
	if err != nil {
		return fmt.Errorf("Query(%q, %q, %q, %q) failed: %v", gerritBaseUrlFlag, username, password, queryStringFlag, err)
	}
	newCLs := newOpenCLs(prevRefs, curQueryResults)
	outputOpenCLs(newCLs)

	// Write current refs to the log file.
	err = writeLog(curQueryResults)
	if err != nil {
		return err
	}

	// Send the new open CLs one by one to the given Jenkins project to run presubmit-test builds.
	newCLsCount := len(newCLs)
	if newCLsCount == 0 {
		return nil
	}
	if jenkinsHostFlag == "" {
		fmt.Println("Not sending CLs to run presubmit tests due to empty Jenkins host.")
		return nil
	}
	sentCount := 0
	for index, curNewCL := range newCLs {
		fmt.Printf("Adding presubmit test build #%d: ", index+1)
		if err := addPresubmitTestBuild(curNewCL); err != nil {
			fmt.Println("FAIL")
			fmt.Fprintf(os.Stderr, "addPresubmitTestBuild(%+v) failed: %v", curNewCL, err)
		} else {
			sentCount++
			fmt.Println("PASS")
		}
	}
	fmt.Printf("%d/%d sent to %s\n", sentCount, newCLsCount, presubmitTestJenkinsProjectFlag)

	return nil
}

// checkGerritBaseUrl performs basic sanity checks for Gerrit base url.
// It returns the gerrit host.
func checkGerritBaseUrl() (string, error) {
	gerritURL, err := url.Parse(gerritBaseUrlFlag)
	if err != nil {
		return "", fmt.Errorf("Parse(%q) failed: %v", gerritBaseUrlFlag, err)
	}
	gerritHost := gerritURL.Host
	if gerritHost == "" {
		return "", fmt.Errorf("%q has no host", gerritBaseUrlFlag)
	}
	return gerritHost, nil
}

// gerritHostCredential returns credential for the given gerritHost.
func gerritHostCredential(gerritHost string) (credential, error) {
	fdNetRc, err := os.Open(netRcFilePathFlag)
	if err != nil {
		return credential{}, fmt.Errorf("Open(%q) failed: %v", netRcFilePathFlag, err)
	}
	defer fdNetRc.Close()
	creds, err := parseNetRcFile(fdNetRc)
	if err != nil {
		return credential{}, err
	}
	gerritCred, ok := creds[gerritHost]
	if !ok {
		return credential{}, fmt.Errorf("cannot find credential for %q in %q", gerritHost, netRcFilePathFlag)
	}
	return gerritCred, nil
}

// parseNetRcFile parses the content of the .netrc file and returns credentials stored in the file indexed by hosts.
func parseNetRcFile(reader io.Reader) (map[string]credential, error) {
	creds := make(map[string]credential)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) != 6 || parts[0] != "machine" || parts[2] != "login" || parts[4] != "password" {
			continue
		}
		creds[parts[1]] = credential{
			username: parts[3],
			password: parts[5],
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	return creds, nil
}

// readLog returns a set of ref strings stored in the log file.
func readLog() (map[string]bool, error) {
	fd, err := os.OpenFile(logFilePathFlag, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("OpenFile(%q) failed: %v", logFilePathFlag, err)
	}
	defer fd.Close()

	// Read file line by line and put the content into a set.
	scanner := bufio.NewScanner(fd)
	refs := make(map[string]bool)
	for scanner.Scan() {
		refs[scanner.Text()] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	return refs, nil
}

// writeLog writes the refs (from the given QueryResult entries) to the log file.
func writeLog(queryResults []gerrit.QueryResult) error {
	fd, err := os.OpenFile(logFilePathFlag, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("OpenFile(%q) failed: %v", logFilePathFlag, err)
	}
	defer fd.Close()

	w := bufio.NewWriter(fd)
	for _, queryResult := range queryResults {
		fmt.Fprintln(w, queryResult.Ref)
	}
	return w.Flush()
}

// newOpenCLs returns the "new" CLs whose refs are not in the CLs from previous query.
// Note that the same CLs with different patch sets have different refs.
func newOpenCLs(prevRefs map[string]bool, curQueryResults []gerrit.QueryResult) []gerrit.QueryResult {
	newCLs := []gerrit.QueryResult{}
	for _, curQueryResult := range curQueryResults {
		if _, ok := prevRefs[curQueryResult.Ref]; !ok {
			newCLs = append(newCLs, curQueryResult)
		}
	}
	return newCLs
}

// outputOpenCLs prints out the given QueryResult entries line by line.
// Each line shows the link to the CL and its related info.
func outputOpenCLs(queryResults []gerrit.QueryResult) {
	if len(queryResults) == 0 {
		fmt.Println("No new open CLs")
		return
	}
	count := len(queryResults)
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "%d new open CL", count)
	if count > 1 {
		fmt.Fprintf(buf, "s")
	}
	fmt.Println(buf.String())
	for _, queryResult := range queryResults {
		// The ref string is in the form of /refs/12/3412/1 where "3412" is the CL number and "1" is the patch set number.
		parts := strings.Split(queryResult.Ref, "/")
		fmt.Printf("http://go/vcl/%s [PatchSet: %s, Repo: %s]\n", parts[3], parts[4], queryResult.Repo)
	}
}

// addPresubmitTestBuild uses Jenkins' remote access API to add a build for a given open CL to run presubmit tests.
func addPresubmitTestBuild(queryResult gerrit.QueryResult) error {
	addBuildUrl, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	addBuildUrl.Path = fmt.Sprintf("%s/job/%s/buildWithParameters", addBuildUrl.Path, presubmitTestJenkinsProjectFlag)
	addBuildUrl.RawQuery = url.Values{
		"token":    {jenkinsTokenFlag},
		"REF":      {queryResult.Ref},
		"REPO":     {queryResult.Repo},
		"CHANGEID": {queryResult.ChangeID},
	}.Encode()
	resp, err := http.Get(addBuildUrl.String())
	if err == nil {
		resp.Body.Close()
	}
	return err
}

// cmdPost represents the 'post' command of the presubmitter tool.
var cmdPost = &cmdline.Command{
	Name:  "post",
	Short: "Post review with the test results to Gerrit",
	Long: `
This subcommand posts review with the test results to Gerrit. It also sets Verified
label to +1.
`,
	Run: runPost,
}

// runPost implements the "post" subcommand.
func runPost(command *cmdline.Command, args []string) error {
	if !strings.HasPrefix(reviewTargetRefFlag, "refs/changes/") {
		return fmt.Errorf("invalid ref: %q", reviewTargetRefFlag)
	}

	// Basic sanity check for the Gerrit base url.
	gerritHost, err := checkGerritBaseUrl()
	if err != nil {
		return err
	}

	// Parse .netrc file to get Gerrit credential.
	gerritCred, err := gerritHostCredential(gerritHost)
	if err != nil {
		return err
	}

	// Construct and post review.
	// TODO(jingjin): do we need to add a flag to the post command to add "Verified" label?
	review := gerrit.GerritReview{
		Message: reviewMessageFlag,
	}
	err = gerrit.PostReview(gerritBaseUrlFlag, gerritCred.username, gerritCred.password, reviewTargetRefFlag, review)
	if err != nil {
		return err
	}

	return nil
}

// cmdSelfUpdate represents the 'selfupdate' command of the presubmitter tool.
var cmdSelfUpdate = &cmdline.Command{
	Run:   runSelfUpdate,
	Name:  "selfupdate",
	Short: "Update the presubmitter tool",
	Long:  "Download and install the latest version of the presubmitter tool.",
}

func runSelfUpdate(command *cmdline.Command, args []string) error {
	return util.SelfUpdate(false, manifestFlag, "presubmitter")
}

// cmdVersion represent the 'version' command of the presubmitter tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the presubmitter tool.",
}

const version string = "0.1"

// commitId should be over-written during build:
// go build -ldflags "-X tools/presubmitter/impl.commitId <commitId>" tools/presubmitter
var commitId string = "test-build"

func runVersion(cmd *cmdline.Command, args []string) error {
	fmt.Printf("presubmitter tool version %v (build %v)\n", version, commitId)
	return nil
}
