package impl

import (
	"fmt"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/gerrit"
)

// cmdPost represents the 'post' command of the presubmit tool.
var cmdPost = &cmdline.Command{
	Name:  "post",
	Short: "Post review with the test results to Gerrit",
	Long:  "This subcommand posts review with the test results to Gerrit.",
	Run:   runPost,
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
	review := gerrit.GerritReview{
		Message: reviewMessageFlag,
	}
	err = gerrit.PostReview(gerritBaseUrlFlag, gerritCred.username, gerritCred.password, reviewTargetRefFlag, review)
	if err != nil {
		return err
	}

	return nil
}
