package main

import (
	"fmt"
	"net/url"
)

func genStartPresubmitBuildLink(strRefs, strProjects, strTests string) string {
	return fmt.Sprintf("%s/%s/buildWithParameters?REFS=%s&PROJECTS=%s&TESTS=%s",
		jenkinsBaseJobUrl,
		presubmitTestJobFlag,
		url.QueryEscape(strRefs),
		url.QueryEscape(strProjects),
		url.QueryEscape(strTests))
}
