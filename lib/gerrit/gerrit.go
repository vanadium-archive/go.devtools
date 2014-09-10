package gerrit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Helpers for formatting the parameters of a change list.
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

// Reference inputs a draft flag, a list of reviewers, a list of ccers, and
// the branch name. It returns a matching string representation of a Gerrit
// reference.
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

// QueryResult contains the essential data we care about from query results.
type QueryResult struct {
	Ref      string
	Repo     string
	ChangeID string
}

// Query returns a list of QueryResult entries matched by the given Gerrit query
// string from the given Gerrit instance. The result is sorted by the last update
// time, most recently updated to oldest updated.
//
// See the following links for more details about Gerrit search syntax:
// - https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
// - https://gerrit-review.googlesource.com/Documentation/user-search.html
func Query(gerritBaseUrl, username, password, queryString string) ([]QueryResult, error) {
	url := fmt.Sprintf("%s/a/changes/?o=CURRENT_REVISION&q=%s", gerritBaseUrl, url.QueryEscape(queryString))
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

	return parseQueryResults(res.Body)
}

// parseQueryResults parses a list of Gerrit ChangeInfo entries (json result of a query)
// and returns a list of QueryResult entries.
func parseQueryResults(reader io.Reader) ([]QueryResult, error) {
	r := bufio.NewReader(reader)

	// The first line of the input is the XSSI guard ")]}'". Getting rid of that.
	if _, err := r.ReadSlice('\n'); err != nil {
		return nil, err
	}

	// Parse the remaining ChangeInfo entries and extract data to construct the QueryResult
	// slice to return.
	var changes []struct {
		Change_id        string
		Current_revision string
		Project          string
		Revisions        map[string]struct {
			Fetch struct {
				Http struct {
					Ref string
				}
			}
		}
	}
	if err := json.NewDecoder(r).Decode(&changes); err != nil {
		return nil, fmt.Errorf("Decode() failed: %v", err)
	}

	var refs []QueryResult
	for _, change := range changes {
		refs = append(refs, QueryResult{
			Ref:      change.Revisions[change.Current_revision].Fetch.Http.Ref,
			Repo:     change.Project,
			ChangeID: change.Change_id,
		})
	}
	return refs, nil
}
