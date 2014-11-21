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

	"veyron.io/tools/lib/gitutil"
	"veyron.io/tools/lib/util"
)

var (
	remoteRE    = regexp.MustCompile("remote:[^\n]*")
	multiPartRE = regexp.MustCompile(`MultiPart:\s*(\d+)\s*/\s*(\d+)`)
)

// Comment represents a single inline file comment.
type Comment struct {
	Line    int    `json:"line,omitempty"`
	Message string `json:"message,omitempty"`
}

// GerritReview represents a Gerrit review.
// For more details, see:
// http://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#review-input
type GerritReview struct {
	Message  string               `json:"message,omitempty"`
	Labels   struct{}             `json:"labels,omitempty"`
	Comments map[string][]Comment `json:"comments,omitempty"`
}

// PostReview posts a review to the given Gerrit ref.
func PostReview(ctx *util.Context, gerritBaseUrl, username, password, ref string, review GerritReview) error {
	fmt.Fprintf(ctx.Stdout(), "Posting review for %s:\n%#v\n", ref, review)

	// Construct api url.
	// ref is in the form of "refs/changes/<last two digits of change number>/<change number>/<patch set number>".
	parts := strings.Split(ref, "/")
	if expected, got := 5, len(parts); expected != got {
		return fmt.Errorf("unexpected number of %q parts: expected %v, got %v", ref, expected, got)
	}
	cl, revision := parts[3], parts[4]
	url := fmt.Sprintf("%s/a/changes/%s/revisions/%s/review", gerritBaseUrl, cl, revision)

	// Encode "review" in json.
	encodedBytes, err := json.Marshal(review)
	if err != nil {
		return fmt.Errorf("Marshal(%#v) failed: %v", review, err)
	}

	// Post review.
	method, body := "POST", bytes.NewReader(encodedBytes)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Content-Type", "application/json;charset=UTF-8")
	req.SetBasicAuth(username, password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	res.Body.Close()

	return nil
}

// QueryResult contains the essential data we care about from query
// results.
type QueryResult struct {
	Ref       string
	Repo      string
	ChangeID  string
	MultiPart *MultiPartCLInfo
}

// MultiPartCLInfo contains data used to process multiple cls across different repos.
type MultiPartCLInfo struct {
	Topic string
	Index int // This should be 1-based.
	Total int
}

// parseQueryResults parses a list of Gerrit ChangeInfo entries (json
// result of a query) and returns a list of QueryResult entries.
func parseQueryResults(ctx *util.Context, reader io.Reader) ([]QueryResult, error) {
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
		}
		multiPartCLInfo, err := parseMultiPartMatch(change.Revisions[change.Current_revision].Commit.Message)
		if err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			continue
		}
		if multiPartCLInfo != nil {
			multiPartCLInfo.Topic = change.Topic
		}
		queryResult.MultiPart = multiPartCLInfo
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

// Query returns a list of QueryResult entries matched by the given
// Gerrit query string from the given Gerrit instance. The result is
// sorted by the last update time, most recently updated to oldest
// updated.
//
// See the following links for more details about Gerrit search syntax:
// - https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
// - https://gerrit-review.googlesource.com/Documentation/user-search.html
func Query(ctx *util.Context, gerritBaseUrl, username, password, queryString string) ([]QueryResult, error) {
	url := fmt.Sprintf("%s/a/changes/?o=CURRENT_REVISION&o=CURRENT_COMMIT&q=%s", gerritBaseUrl, url.QueryEscape(queryString))
	fmt.Fprintf(ctx.Stdout(), "Issuing query: %v\n", url)
	var body io.Reader
	method, body := "GET", nil
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("NewRequest(%q, %q, %v) failed: %v", method, url, body, err)
	}
	req.Header.Add("Accept", "application/json")
	req.SetBasicAuth(username, password)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Do(%v) failed: %v", req, err)
	}
	defer res.Body.Close()

	return parseQueryResults(ctx, res.Body)
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

// repoName returns the URL of the veyron Gerrit repository with
// respect to the repository identified by the current working
// directory.
func repoName(ctx *util.Context) (string, error) {
	args := []string{"config", "--get", "remote.origin.url"}
	var stdout, stderr bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if err := ctx.Run().CommandWithOpts(opts, "git", args...); err != nil {
		return "", gitutil.Error(stdout.String(), stderr.String(), args...)
	}
	return "https://veyron-review.googlesource.com/" + filepath.Base(strings.TrimSpace(stdout.String())), nil
}

// Review pushes the branch to Gerrit.
func Review(ctx *util.Context, repoPathArg string, draft bool, reviewers, ccs, branch string) error {
	repoPath := repoPathArg
	if repoPathArg == "" {
		var err error
		repoPath, err = repoName(ctx)
		if err != nil {
			return err
		}
	}
	refspec := "HEAD:" + Reference(draft, reviewers, ccs, branch)
	args := []string{"push", repoPath, refspec}
	var stdout, stderr bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if err := ctx.Run().CommandWithOpts(opts, "git", args...); err != nil {
		return gitutil.Error(stdout.String(), stderr.String(), args...)
	}
	for _, line := range strings.Split(stderr.String(), "\n") {
		if remoteRE.MatchString(line) {
			fmt.Println(line)
		}
	}
	return nil
}
