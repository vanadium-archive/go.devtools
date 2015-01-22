// Package gerrit provides library functions for interacting with the
// gerrit code review system.
package gerrit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"v.io/tools/lib/collect"
	"v.io/tools/lib/gitutil"
	"v.io/tools/lib/runutil"
)

var (
	remoteRE        = regexp.MustCompile("remote:[^\n]*")
	multiPartRE     = regexp.MustCompile(`MultiPart:\s*(\d+)\s*/\s*(\d+)`)
	presubmitTestRE = regexp.MustCompile(`PresubmitTest:\s*(.*)`)
)

// Comment represents a single inline file comment.
type Comment struct {
	Line    int    `json:"line,omitempty"`
	Message string `json:"message,omitempty"`
}

// Review represents a Gerrit review. For more details, see:
// http://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#review-input
type Review struct {
	Message  string               `json:"message,omitempty"`
	Labels   map[string]string    `json:"labels,omitempty"`
	Comments map[string][]Comment `json:"comments,omitempty"`
}

type Gerrit struct {
	host     string
	password string
	username string
}

// New is the Gerrit factory.
func New(host, username, password string) *Gerrit {
	return &Gerrit{
		host:     host,
		password: password,
		username: username,
	}
}

// PostReview posts a review to the given Gerrit reference.
func (g *Gerrit) PostReview(ref string, message string, labels map[string]string) (e error) {
	review := Review{
		Message: message,
		Labels:  labels,
	}

	// Encode "review" as JSON.
	encodedBytes, err := json.Marshal(review)
	if err != nil {
		return fmt.Errorf("Marshal(%#v) failed: %v", review, err)
	}

	// Construct API URL.
	// ref is in the form of "refs/changes/<last two digits of change number>/<change number>/<patch set number>".
	parts := strings.Split(ref, "/")
	if expected, got := 5, len(parts); expected != got {
		return fmt.Errorf("unexpected number of %q parts: expected %v, got %v", ref, expected, got)
	}
	cl, revision := parts[3], parts[4]
	url := fmt.Sprintf("%s/a/changes/%s/revisions/%s/review", g.host, cl, revision)

	// Post the review.
	method, body := "POST", bytes.NewReader(encodedBytes)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Content-Type", "application/json;charset=UTF-8")
	req.SetBasicAuth(g.username, g.password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer collect.Error(func() error { return res.Body.Close() }, &e)

	return nil
}

// QueryResult represents query result data we care about.
type QueryResult struct {
	ChangeID      string
	Labels        map[string]struct{}
	MultiPart     *MultiPartCLInfo
	PresubmitTest PresubmitTestType
	Ref           string
	Repo          string
}

type PresubmitTestType string

const (
	PresubmitTestTypeNone PresubmitTestType = "none"
	PresubmitTestTypeAll  PresubmitTestType = "all"
)

func PresubmitTestTypes() []string {
	return []string{string(PresubmitTestTypeNone), string(PresubmitTestTypeAll)}
}

// MultiPartCLInfo contains data used to process multiple cls across
// different repos.
type MultiPartCLInfo struct {
	Topic string
	Index int // This should be 1-based.
	Total int
}

// parseQueryResults parses a list of Gerrit ChangeInfo entries (json
// result of a query) and returns a list of QueryResult entries.
func parseQueryResults(reader io.Reader) ([]QueryResult, error) {
	r := bufio.NewReader(reader)

	// The first line of the input is the XSSI guard
	// ")]}'". Getting rid of that.
	if _, err := r.ReadSlice('\n'); err != nil {
		return nil, err
	}

	// Parse the remaining ChangeInfo entries and extract data to
	// construct the QueryResult slice to return.
	var changes []struct {
		Change_id        string
		Current_revision string
		Project          string
		Topic            string
		Revisions        map[string]struct {
			Fetch struct {
				Http struct {
					Ref string
				}
			}
			Commit struct {
				Message string // This contains both "subject" and the rest of the commit message.
			}
		}
		Labels map[string]struct{}
	}
	if err := json.NewDecoder(r).Decode(&changes); err != nil {
		return nil, fmt.Errorf("Decode() failed: %v", err)
	}

	var refs []QueryResult
	for _, change := range changes {
		queryResult := QueryResult{
			Ref:      change.Revisions[change.Current_revision].Fetch.Http.Ref,
			Repo:     change.Project,
			ChangeID: change.Change_id,
			Labels:   change.Labels,
		}
		clMessage := change.Revisions[change.Current_revision].Commit.Message
		multiPartCLInfo, err := parseMultiPartMatch(clMessage)
		if err != nil {
			return nil, err
		}
		if multiPartCLInfo != nil {
			multiPartCLInfo.Topic = change.Topic
		}
		queryResult.MultiPart = multiPartCLInfo
		presubmitType := parsePresubmitTestType(clMessage)
		queryResult.PresubmitTest = presubmitType
		refs = append(refs, queryResult)
	}
	return refs, nil
}

// parseMultiPartMatch uses multiPartRE (a pattern like: MultiPart: 1/3) to match the given string.
func parseMultiPartMatch(match string) (*MultiPartCLInfo, error) {
	matches := multiPartRE.FindStringSubmatch(match)
	if matches != nil {
		index, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("Atoi(%q) failed: %v", matches[1], err)
		}
		total, err := strconv.Atoi(matches[2])
		if err != nil {
			return nil, fmt.Errorf("Atoi(%q) failed: %v", matches[2], err)
		}
		return &MultiPartCLInfo{
			Index: index,
			Total: total,
		}, nil
	}
	return nil, nil
}

// parsePresubmitTestType uses presubmitTestRE to match the given string and
// returns the presubmit test type.
func parsePresubmitTestType(match string) PresubmitTestType {
	ret := PresubmitTestTypeAll
	matches := presubmitTestRE.FindStringSubmatch(match)
	if matches != nil {
		switch matches[1] {
		case string(PresubmitTestTypeNone):
			ret = PresubmitTestTypeNone
		case string(PresubmitTestTypeAll):
			ret = PresubmitTestTypeAll
		}
	}
	return ret
}

// Query returns a list of QueryResult entries matched by the given
// Gerrit query string from the given Gerrit instance. The result is
// sorted by the last update time, most recently updated to oldest
// updated.
//
// See the following links for more details about Gerrit search syntax:
// - https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
// - https://gerrit-review.googlesource.com/Documentation/user-search.html
func (g *Gerrit) Query(query string) (_ []QueryResult, e error) {
	url := fmt.Sprintf("%s/a/changes/?o=CURRENT_REVISION&o=CURRENT_COMMIT&o=LABELS&q=%s", g.host, url.QueryEscape(query))
	var body io.Reader
	method, body := "GET", nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	req.SetBasicAuth(g.username, g.password)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer collect.Error(func() error { return res.Body.Close() }, &e)
	return parseQueryResults(res.Body)
}

// formatParams formats parameters of a change list.
func formatParams(params, key string, email bool) []string {
	if len(params) == 0 {
		return []string{}
	}
	paramsSlice := strings.Split(params, ",")
	formattedParamsSlice := make([]string, len(paramsSlice))
	for i, param := range paramsSlice {
		value := strings.TrimSpace(param)
		if !strings.Contains(value, "@") && email {
			// Param is only an ldap and we need an email;
			// append @google.com to it.
			value = value + "@google.com"
		}
		formattedParamsSlice[i] = key + "=" + value
	}
	return formattedParamsSlice
}

// Reference inputs a draft flag, a list of reviewers, a list of
// ccers, and the branch name. It returns a matching string
// representation of a Gerrit reference.
func Reference(draft bool, reviewers, ccs, branch string) string {
	var ref string
	if draft {
		ref = "refs/drafts/master"
	} else {
		ref = "refs/for/master"
	}

	params := formatParams(reviewers, "r", true)
	params = append(params, formatParams(ccs, "cc", true)...)
	params = append(params, formatParams(branch, "topic", false)...)

	if len(params) > 0 {
		ref = ref + "%" + strings.Join(params, ",")
	}

	return ref
}

// repoName returns the URL of the vanadium Gerrit repository with
// respect to the repository identified by the current working
// directory.
func repoName(run *runutil.Run) (string, error) {
	args := []string{"config", "--get", "remote.origin.url"}
	var stdout, stderr bytes.Buffer
	opts := run.Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if err := run.CommandWithOpts(opts, "git", args...); err != nil {
		return "", gitutil.Error(stdout.String(), stderr.String(), args...)
	}
	return "https://vanadium-review.googlesource.com/" + filepath.Base(strings.TrimSpace(stdout.String())), nil
}

// Push pushes the current branch to Gerrit.
func Push(run *runutil.Run, repoPathArg string, draft bool, reviewers, ccs, branch string) error {
	repoPath := repoPathArg
	if repoPathArg == "" {
		var err error
		repoPath, err = repoName(run)
		if err != nil {
			return err
		}
	}
	refspec := "HEAD:" + Reference(draft, reviewers, ccs, branch)
	args := []string{"push", repoPath, refspec}
	var stdout, stderr bytes.Buffer
	opts := run.Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if err := run.CommandWithOpts(opts, "git", args...); err != nil {
		return gitutil.Error(stdout.String(), stderr.String(), args...)
	}
	for _, line := range strings.Split(stderr.String(), "\n") {
		if remoteRE.MatchString(line) {
			fmt.Println(line)
		}
	}
	return nil
}
