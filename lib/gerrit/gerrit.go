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

func formatReviewerParams(reviewers string) []string {
	return formatParams(reviewers, "r")
}

func formatCCParams(ccs string) []string {
	return formatParams(ccs, "cc")
}

// Reference inputs a draft flag and a list of reviewers and ccers and
// returns a matching string representation of a Gerrit reference.
func Reference(draft bool, reviewers, ccs string) string {
	var ref string
	if draft {
		ref = "refs/drafts/master"
	} else {
		ref = "refs/for/master"
	}

	reviewerParams := formatReviewerParams(reviewers)
	ccParams := formatCCParams(ccs)
	allParams := append(reviewerParams, ccParams...)

	if len(allParams) > 0 {
		ref = ref + "%" + strings.Join(allParams, ",")
	}

	return ref
}
