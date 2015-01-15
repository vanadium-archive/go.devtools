package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"v.io/lib/cmdline"
	"v.io/tools/lib/collect"
	"v.io/tools/lib/gerrit"
	"v.io/tools/lib/util"
)

type clList []gerrit.QueryResult

// clRefMap indexes cls by their ref strings.
type clRefMap map[string]gerrit.QueryResult

// clNumberToPatchsetMap is a map from CL numbers to the latest patchset of the CL.
type clNumberToPatchsetMap map[int]int

// multiPartCLSet represents a set of CLs that spans multiple repositories.
type multiPartCLSet struct {
	parts         map[int]gerrit.QueryResult // Indexed by cl's part index.
	expectedTotal int
	expectedTopic string
}

// NewMultiPartCLSet creates a new instance of multiPartCLSet.
func NewMultiPartCLSet() *multiPartCLSet {
	return &multiPartCLSet{
		parts:         map[int]gerrit.QueryResult{},
		expectedTotal: -1,
		expectedTopic: "",
	}
}

// addCL adds a CL to the set after it passes a series of checks.
func (s *multiPartCLSet) addCL(cl gerrit.QueryResult) error {
	if cl.MultiPart == nil {
		return fmt.Errorf("no multi part info found: %#v", cl)
	}
	multiPartInfo := cl.MultiPart
	if s.expectedTotal < 0 {
		s.expectedTotal = multiPartInfo.Total
	}
	if s.expectedTopic == "" {
		s.expectedTopic = multiPartInfo.Topic
	}
	if s.expectedTotal != multiPartInfo.Total {
		return fmt.Errorf("inconsistent total number of cls in this set: want %d, got %d", s.expectedTotal, multiPartInfo.Total)
	}
	if s.expectedTopic != multiPartInfo.Topic {
		return fmt.Errorf("inconsistent cl topics in this set: want %d, got %d", s.expectedTopic, multiPartInfo.Topic)
	}
	if existingCL, ok := s.parts[multiPartInfo.Index]; ok {
		return fmt.Errorf("duplicated cl part %d found:\ncl to add: %v\nexisting cl:%v", multiPartInfo.Index, cl, existingCL)
	}
	s.parts[multiPartInfo.Index] = cl
	return nil
}

// complete returns whether the current set has all the cl parts it needs.
func (s *multiPartCLSet) complete() bool {
	return len(s.parts) == s.expectedTotal
}

// cls returns a list of CLs in this set sorted by their part number.
func (s *multiPartCLSet) cls() clList {
	ret := clList{}
	sortedKeys := []int{}
	for part := range s.parts {
		sortedKeys = append(sortedKeys, part)
	}
	sort.Ints(sortedKeys)
	for _, part := range sortedKeys {
		ret = append(ret, s.parts[part])
	}
	return ret
}

// cmdQuery represents the 'query' command of the presubmit tool.
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
func runQuery(command *cmdline.Command, args []string) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	numSentCLs := 0
	defer func() {
		printf(ctx.Stdout(), "%d sent.\n", numSentCLs)
	}()

	// Basic sanity check for the Gerrit base url.
	gerritHost, err := checkGerritBaseUrl()
	if err != nil {
		return err
	}

	// Don't query anything if the last "presubmit-test" build failed.
	lastStatus, err := lastCompletedBuildStatus(presubmitTestFlag, "")
	if err != nil {
		fmt.Fprintf(ctx.Stderr(), "%v\n", err)
	} else {
		if lastStatus == "FAILURE" {
			printf(ctx.Stdout(), "%s is failing. Skipping this round.\n", presubmitTestFlag)
			return nil
		}
	}

	// Parse .netrc file to get Gerrit credential.
	gerritCred, err := gerritHostCredential(gerritHost)
	if err != nil {
		return err
	}

	// Read previous CLs from the log file.
	prevCLsMap, err := readLog()
	if err != nil {
		return err
	}

	// Query Gerrit.
	username, password := gerritCred.username, gerritCred.password
	curCLs, err := gerrit.Query(ctx, gerritBaseUrlFlag, username, password, queryStringFlag)
	if err != nil {
		return fmt.Errorf("Query(%q, %q, %q, %q) failed: %v", gerritBaseUrlFlag, username, password, queryStringFlag, err)
	}

	// Write current CLs to the log file.
	err = writeLog(ctx, curCLs)
	if err != nil {
		return err
	}

	// Don't send anything if jenkins host is not specified.
	if jenkinsHostFlag == "" {
		printf(ctx.Stdout(), "Not sending CLs to run presubmit tests due to empty Jenkins host.\n")
		return nil
	}

	// Don't send anything if prevCLsMap is empty.
	if len(prevCLsMap) == 0 {
		printf(ctx.Stdout(), "Not sending CLs to run presubmit tests due to empty log file.\n")
		return nil
	}

	// Get new clLists.
	newCLLists := newOpenCLs(ctx, prevCLsMap, curCLs)

	// Send the new open CLs one by one to the given Jenkins
	// project to run presubmit-test builds.
	defaultProjects, _, err := util.ReadManifest(ctx, "default")
	if err != nil {
		return err
	}
	numSentCLs += sendCLListsToPresubmitTest(ctx, newCLLists, defaultProjects, removeOutdatedBuilds, addPresubmitTestBuild)

	return nil
}

// checkGerritBaseUrl performs basic sanity checks for Gerrit base
// url. It returns the gerrit host.
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
func gerritHostCredential(gerritHost string) (_ credential, e error) {
	fdNetRc, err := os.Open(netRcFilePathFlag)
	if err != nil {
		return credential{}, fmt.Errorf("Open(%q) failed: %v", netRcFilePathFlag, err)
	}
	defer collect.Error(func() error { return fdNetRc.Close() }, &e)
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

// parseNetRcFile parses the content of the .netrc file and returns
// credentials stored in the file indexed by hosts.
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

// readLog returns CLs indexed by thier refs stored in the log file.
func readLog() (clRefMap, error) {
	results := clRefMap{}
	bytes, err := ioutil.ReadFile(logFilePathFlag)
	if err != nil {
		if os.IsNotExist(err) {
			return results, nil
		}
		return nil, fmt.Errorf("ReadFile(%q) failed: %v", logFilePathFlag, err)
	}

	if err := json.Unmarshal(bytes, &results); err != nil {
		return nil, fmt.Errorf("Unmarshal failed: %v\n%v", err, string(bytes))
	}
	return results, nil
}

// writeLog writes the refs of the given CLs to the log file.
func writeLog(ctx *util.Context, cls clList) (e error) {
	// Index CLs with their refs.
	results := clRefMap{}
	for _, cl := range cls {
		results[cl.Ref] = cl
	}

	fd, err := os.OpenFile(logFilePathFlag, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("OpenFile(%q) failed: %v", logFilePathFlag, err)
	}
	defer collect.Error(func() error { return fd.Close() }, &e)

	bytes, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent(%v) failed: %v", results, err)
	}
	if err := ctx.Run().WriteFile(logFilePathFlag, bytes, os.FileMode(0644)); err != nil {
		return fmt.Errorf("WriteFile(%q) failed: %v", logFilePathFlag, err)
	}
	return nil
}

// newOpenCLs returns a slice of clLists that are "newer" relative to the
// previous query. A clList is newer if one of the following condition holds:
// - If a clList has only one cl, then it is newer if:
//   * Its ref string cannot be found among the CLs from the previous query.
//
//   For example: from the previous query, we got cl 1000/1 (cl number 1000 and
//   patchset 1). Then clLists [1000/2] and [2000/1] are both newer.
//
// - If a clList has multiple CLs, then it is newer if:
//   * It forms a "consistent" (its CLs have the same topic) and "complete"
//     (it contains all the parts) multi-part CL set.
//   * At least one of their ref strings cannot be found in the CLs from the
//     previous query.
//
//   For example: from the previous query, we got cl 3001/1 which is the first
//   part of a multi part cl set with topic "T1". Suppose the current query
//   returns cl 3002/1 which is the second part of the same set. In this case,
//   a clList [3001/1 3002/1] will be returned. Then suppose in the next query,
//   we got cl 3002/2 which is newer then 3002/1. In this case, a clList
//   [3001/1 3002/2] will be returned.
func newOpenCLs(ctx *util.Context, prevCLsMap clRefMap, curCLs clList) []clList {
	newCLs := []clList{}
	topicsInNewCLs := map[string]struct{}{}
	multiPartCLs := clList{}
	for _, curCL := range curCLs {
		// Ref could be empty in cases where a patchset is causing conflicts.
		if curCL.Ref == "" {
			continue
		}
		if _, ok := prevCLsMap[curCL.Ref]; !ok {
			// This individual cl is newer.
			if curCL.MultiPart == nil {
				// This cl is not a multi part cl.
				// Add it to the return slice.
				newCLs = append(newCLs, clList{curCL})
			} else {
				// This cl is a multi part cl.
				// Record its topic.
				topicsInNewCLs[curCL.MultiPart.Topic] = struct{}{}
			}
		}
		// Record all multi part CLs.
		if curCL.MultiPart != nil {
			multiPartCLs = append(multiPartCLs, curCL)
		}
	}

	// Find complete multi part cl sets.
	setMap := map[string]*multiPartCLSet{}
	for _, curCL := range multiPartCLs {
		multiPartInfo := curCL.MultiPart

		// Skip topics that contain no new CLs.
		topic := multiPartInfo.Topic
		if _, ok := topicsInNewCLs[topic]; !ok {
			continue
		}

		if _, ok := setMap[topic]; !ok {
			setMap[topic] = NewMultiPartCLSet()
		}
		curSet := setMap[topic]
		if err := curSet.addCL(curCL); err != nil {
			printf(ctx.Stderr(), "%v\n", err)
		}
	}
	for _, set := range setMap {
		if set.complete() {
			newCLs = append(newCLs, set.cls())
		}
	}

	return newCLs
}

// sendCLListsToPresubmitTest sends the given clLists to presubmit-test Jenkins
// target one by one to run presubmit-test builds. It returns how many CLs have
// been sent successfully.
func sendCLListsToPresubmitTest(ctx *util.Context, clLists []clList, defaultProjects map[string]util.Project,
	removeOutdatedFn func(*util.Context, clNumberToPatchsetMap) []error,
	addPresubmitFn func(*util.Context, clList) error) int {
	clsSent := 0
outer:
	for _, curCLList := range clLists {
		// Check and cancel matched outdated builds.
		curCLMap := clNumberToPatchsetMap{}
		clStrings := []string{}
		for _, curCL := range curCLList {
			// Ignore all CLs that are not in the default manifest.
			// TODO(jingjin): find a better way so we can remove this check.
			if defaultProjects != nil && !isInDefaultManifest(ctx, curCL, defaultProjects) {
				continue outer
			}

			cl, patchset, err := parseRefString(curCL.Ref)
			if err != nil {
				printf(ctx.Stderr(), "%v\n", err)
				continue outer
			}
			curCLMap[cl] = patchset
			clStrings = append(clStrings, fmt.Sprintf("http://go/vcl/%d/%d", cl, patchset))
		}
		for err := range removeOutdatedFn(ctx, curCLMap) {
			printf(ctx.Stderr(), "%v\n", err)
		}

		// Send curCLList to presubmit-test.
		strCLs := fmt.Sprintf("Add %s", strings.Join(clStrings, ", "))
		if err := addPresubmitFn(ctx, curCLList); err != nil {
			printf(ctx.Stdout(), "FAIL: %s\n", strCLs)
			printf(ctx.Stderr(), "addPresubmitTestBuild(%+v) failed: %v", curCLList, err)
		} else {
			printf(ctx.Stdout(), "PASS: %s\n", strCLs)
			clsSent += len(curCLList)
		}
	}
	return clsSent
}

// isInDefaultManifest checks whether the given cl's repo is in the default manifest.
func isInDefaultManifest(ctx *util.Context, cl gerrit.QueryResult, defaultProjects map[string]util.Project) bool {
	if _, ok := defaultProjects[cl.Repo]; !ok {
		printf(ctx.Stdout(), "project=%q (%s) not found in the default manifest. Skipped.\n", cl.Repo, cl.Ref)
		return false
	}
	return true
}

// removeOutdatedBuilds removes all the outdated presubmit-test builds
// that have the given cl number and equal or smaller patchset
// number. Outdated builds include queued builds and ongoing build.
//
// Since this is not a critical operation, we simply print out the
// errors if we see any.
func removeOutdatedBuilds(ctx *util.Context, cls clNumberToPatchsetMap) (errs []error) {
	// Queued presubmit-test builds.
	getQueuedBuildsRes, err := jenkinsAPI("queue/api/json", "GET", nil)
	if err != nil {
		errs = append(errs, nil)
	} else {
		// Get queued presubmit-test builds.
		defer collect.Errors(func() error { return getQueuedBuildsRes.Body.Close() }, &errs)
		queuedItems, queuedErrs := queuedOutdatedBuilds(getQueuedBuildsRes.Body, cls)
		errs = append(errs, queuedErrs...)

		// Cancel them.
		for _, queuedItem := range queuedItems {
			cancelQueuedItemUri := "queue/cancelItem"
			cancelQueuedItemRes, err := jenkinsAPI(cancelQueuedItemUri, "POST", map[string][]string{
				"id": {fmt.Sprintf("%d", queuedItem.id)},
			})
			if err != nil {
				errs = append(errs, err)
				continue
			} else {
				printf(ctx.Stdout(), "Cancelled build %s as it is no longer current.\n", queuedItem.ref)
				if err := cancelQueuedItemRes.Body.Close(); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	// Ongoing presubmit-test builds.
	getLastBuildUri := fmt.Sprintf("job/%s/lastBuild/api/json", presubmitTestFlag)
	getLastBuildRes, err := jenkinsAPI(getLastBuildUri, "GET", nil)
	if err != nil {
		errs = append(errs, err)
	} else {
		// Get ongoing presubmit-test build.
		defer collect.Errors(func() error { return getLastBuildRes.Body.Close() }, &errs)
		build, err := ongoingOutdatedBuild(getLastBuildRes.Body, cls)
		if err != nil {
			errs = append(errs, err)
			return
		}
		if build.buildNumber < 0 {
			return
		}

		// Cancel it.
		cancelOngoingBuildUri := fmt.Sprintf("job/%s/%d/stop", presubmitTestFlag, build.buildNumber)
		cancelOngoingBuildRes, err := jenkinsAPI(cancelOngoingBuildUri, "POST", nil)
		if err != nil {
			errs = append(errs, err)
		} else {
			printf(ctx.Stdout(), "Cancelled build %s as it is no longer current.\n", build.ref)
			if err := cancelOngoingBuildRes.Body.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

type queuedItem struct {
	id  int
	ref string
}

// queuedOutdatedBuilds returns the ids and refs of queued
// presubmit-test builds that have the given cl number and equal or
// smaller patchset number.
func queuedOutdatedBuilds(reader io.Reader, cls clNumberToPatchsetMap) (_ []queuedItem, errs []error) {
	r := bufio.NewReader(reader)
	var items struct {
		Items []struct {
			Id     int
			Params string `json:"params,omitempty"`
			Task   struct {
				Name string
			}
		}
	}
	if err := json.NewDecoder(r).Decode(&items); err != nil {
		var buf bytes.Buffer
		buf.ReadFrom(reader)
		return nil, []error{fmt.Errorf("Decode() failed: %v\n%s", err, buf.String())}
	}

	queuedItems := []queuedItem{}
	for _, item := range items.Items {
		if item.Task.Name != presubmitTestFlag {
			continue
		}
		// Parse the ref, and append the id/ref of the build
		// if it passes the checks.  The param string is in
		// the form of:
		// "\nREFS=ref/changes/12/3412/2\nREPOS=test" or
		// "\nREPOS=test\nREFS=ref/changes/12/3412/2"
		parts := strings.Split(item.Params, "\n")
		ref := ""
		refPrefix := "REFS="
		for _, part := range parts {
			if strings.HasPrefix(part, refPrefix) {
				ref = strings.TrimPrefix(part, refPrefix)
				break
			}
		}
		if ref == "" {
			errs = append(errs, fmt.Errorf("%s failed to find ref parameter: %q", outputPrefix, item.Params))
			continue
		}
		buildOutdated, err := isBuildOutdated(ref, cls)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if buildOutdated {
			queuedItems = append(queuedItems, queuedItem{
				id:  item.Id,
				ref: ref,
			})
		}
	}

	return queuedItems, errs
}

type ongoingBuild struct {
	buildNumber int
	ref         string
}

// ongoingOutdatedBuild returns the build number/ref of the last
// presubmit build if the build is still ongoing and the build has the
// given cl number and a smaller patchset index.
func ongoingOutdatedBuild(reader io.Reader, cls clNumberToPatchsetMap) (ongoingBuild, error) {
	invalidOngoingBuild := ongoingBuild{buildNumber: -1}

	r := bufio.NewReader(reader)
	var build struct {
		Actions []struct {
			Parameters []struct {
				Name  string
				Value string
			}
		}
		Building bool
		Number   int
	}
	if err := json.NewDecoder(r).Decode(&build); err != nil {
		var buf bytes.Buffer
		buf.ReadFrom(reader)
		return invalidOngoingBuild, fmt.Errorf("Decode() failed: %v\n%s", err, buf.String())
	}

	if !build.Building {
		return invalidOngoingBuild, nil
	}

	// Parse the ref, and return the build number if it passes the checks.
	ref := ""
loop:
	for _, action := range build.Actions {
		for _, param := range action.Parameters {
			if param.Name == "REFS" {
				ref = param.Value
				break loop
			}
		}
	}
	if ref != "" {
		buildOutdated, err := isBuildOutdated(ref, cls)
		if err != nil {
			return invalidOngoingBuild, nil
		}
		if buildOutdated {
			return ongoingBuild{
				buildNumber: build.Number,
				ref:         ref,
			}, nil
		} else {
			return invalidOngoingBuild, nil
		}
	}

	return ongoingBuild{}, fmt.Errorf("%s failed to find ref string", outputPrefix)
}

// isBuildOutdated checks whether a build (identified by the given refs string)
// is older than the cls in newCLs.
// Note that curRefs may contain multiple ref strings separated by ":".
func isBuildOutdated(curRefs string, newCLs clNumberToPatchsetMap) (bool, error) {
	// Parse the refs string into a clNumberToPatchsetMap object.
	curCLs := clNumberToPatchsetMap{}
	refs := strings.Split(curRefs, ":")
	for _, ref := range refs {
		cl, patchset, err := parseRefString(ref)
		if err != nil {
			return false, err
		}
		curCLs[cl] = patchset
	}

	// Check curCLs and newCLs have the same set of cl numbers.
	newCLNumbers := sortedKeys(newCLs)
	if !reflect.DeepEqual(sortedKeys(curCLs), newCLNumbers) {
		return false, nil
	}

	// Check patchsets.
	outdated := true
	for _, clNumber := range newCLNumbers {
		curPatchset := curCLs[clNumber]
		newPatchset := newCLs[clNumber]
		if newPatchset < curPatchset {
			outdated = false
			break
		}
	}
	return outdated, nil
}

func sortedKeys(cls clNumberToPatchsetMap) []int {
	keys := []int{}
	for k := range cls {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// parseRefString parses the cl and patchset number from the given ref string.
func parseRefString(ref string) (int, int, error) {
	parts := strings.Split(ref, "/")
	if expected, got := 5, len(parts); expected != got {
		return -1, -1, fmt.Errorf("unexpected number of %q parts: expected %v, got %v", ref, expected, got)
	}
	cl, err := strconv.Atoi(parts[3])
	if err != nil {
		return -1, -1, fmt.Errorf("Atoi(%q) failed: %v", parts[3], err)
	}
	patchset, err := strconv.Atoi(parts[4])
	if err != nil {
		return -1, -1, fmt.Errorf("Atoi(%q) failed: %v", parts[4], err)
	}
	return cl, patchset, nil
}

// jenkinsAPI calls the given REST API uri and gets the json response if available.
func jenkinsAPI(uri, method string, params map[string][]string) (*http.Response, error) {
	// Construct url.
	apiURL, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return nil, fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	apiURL.Path = fmt.Sprintf("%s/%s", apiURL.Path, uri)
	values := url.Values{
		"token": {jenkinsTokenFlag},
	}
	if params != nil {
		for name := range params {
			values[name] = params[name]
		}
	}
	apiURL.RawQuery = values.Encode()

	// Get response.
	var body io.Reader
	url, body := apiURL.String(), nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	return res, nil
}

// addPresubmitTestBuild uses Jenkins' remote access API to add a build for
// a set of open CLs to run presubmit tests.
func addPresubmitTestBuild(ctx *util.Context, cls clList) error {
	addBuildUrl, err := url.Parse(jenkinsHostFlag)
	if err != nil {
		return fmt.Errorf("Parse(%q) failed: %v", jenkinsHostFlag, err)
	}
	refs, repos, fullRepos := []string{}, []string{}, []string{}
	for _, cl := range cls {
		refs = append(refs, cl.Ref)
		repos = append(repos, cl.Repo)
		fullRepos = append(fullRepos, util.VanadiumGitRepoHost()+cl.Repo)
	}

	// Get tests to run.
	var config util.Config
	if err := util.LoadConfig("common", &config); err != nil {
		return err
	}
	tests := config.ProjectTests(fullRepos)

	addBuildUrl.Path = fmt.Sprintf("%s/job/%s/buildWithParameters", addBuildUrl.Path, presubmitTestFlag)
	addBuildUrl.RawQuery = url.Values{
		"token": {jenkinsTokenFlag},
		"REFS":  {strings.Join(refs, ":")},
		"REPOS": {strings.Join(repos, ":")},
		// Separating by spaces is required by the Dynamic Axis plugin used in the
		// new presubmit test target.
		"TESTS": {strings.Join(tests, " ")},
	}.Encode()
	resp, err := http.Get(addBuildUrl.String())
	if err == nil {
		resp.Body.Close()
	}
	return err
}
