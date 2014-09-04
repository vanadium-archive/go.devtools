package gerrit

import (
	"strings"
)

// Helpers for formatting the reviewers and CCs on a change list.
func formatParams(params, keyName string) []string {
	if len(params) == 0 {
		return []string{}
	}

	paramsSlice := strings.Split(params, ",")
	formattedParamsSlice := make([]string, len(paramsSlice))

	for i, param := range paramsSlice {
		trimmedParam := strings.TrimSpace(param)
		var email string
		if strings.Contains(trimmedParam, "@") {
			// Param is already an email.
			email = trimmedParam
		} else {
			// Param is only an ldap.
			email = trimmedParam + "@google.com"
		}
		formattedParamsSlice[i] = keyName + "=" + email
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

	params := formatParams(reviewers, "r")
	params = append(params, formatParams(ccs, "cc")...)
	params = append(params, formatParams(branch, "topic")...)

	if len(params) > 0 {
		ref = ref + "%" + strings.Join(params, ",")
	}

	return ref
}
