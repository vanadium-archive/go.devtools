package impl

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"tools/git"
	"veyron/lib/cmdline"
)

var (
	draft     bool
	reviewers string
	ccs       string
)

// init carries out the package initialization.
func init() {
	cmdReview.Flags.BoolVar(&draft, "d", false, "Send draft change list.")
	cmdReview.Flags.StringVar(&reviewers, "r", "", "Comma-seperated list of emails or LDAPs to request review.")
	cmdReview.Flags.StringVar(&ccs, "cc", "", "Comma-seperated list of emails or LDAPs to cc.")
}

// Root returns a command that represents the root of the review tool.
func Root() *cmdline.Command {
	return &cmdline.Command{
		Name:  "veyron",
		Short: "Command-line tool for interacting with the Veyron Gerrit server",
		Long: `
The veyron tool facilitates interaction with the Veyron Gerrit server.
In particular, it can be used to export changes from a local branch
to the Gerrit server.
`,
		Children: []*cmdline.Command{cmdReview},
	}
}

// cmdReview represent the 'review' command of the review tool.
var cmdReview = &cmdline.Command{
	Run:   runReview,
	Name:  "review",
	Short: "Send changes from a local branch to Gerrit for review",
	Long: `
Squashes all commits of a local branch into a single commit and
submits that commit to Gerrit as a single change list.  You can run
it multiple times to send more patch sets to the change list.
`,
}

var errOperationFailed = errors.New("operation failed")

type EmptyChangeError string

func (s EmptyChangeError) Error() string {
	return fmt.Sprintf("No commits on branch %s.", string(s))
}

type NoChangeIdError string

func (s NoChangeIdError) Error() string {
	return fmt.Sprintf("No Change-Id in commit %s.", string(s))
}

// runReview is a wrapper that sets up and runs a review instance.
func runReview(cmd *cmdline.Command, args []string) error {
	branch, err := git.CurrentBranchName()
	if err != nil {
		fmt.Errorf("git.CurrentBranchName() failed: %v", err)
		return errOperationFailed
	}
	edit, repo := true, ""
	r := NewReview(draft, edit, branch, repo, reviewers, ccs)
	return r.run()
}

// review holds the state of a review.
type review struct {
	// branch is the name of the git branch from which the review is created.
	branch string
	// ccs is the list of LDAPs or emails to cc on the review.
	ccs string
	// draft indicates whether to create a draft review.
	draft bool
	// edit indicates whether to edit the review message.
	edit bool
	// repo is the name of the gerrit repository.
	repo string
	// reviewBranch is the name of the temporary git branch used to send the review.
	reviewBranch string
	// reviewers is the list of LDAPs or emails to request a review from.
	reviewers string
}

// NewReview is the review factory.
func NewReview(draft, edit bool, branch, repo, reviewers, ccs string) *review {
	reviewBranch := branch + "-REVIEW"
	return &review{
		branch:       branch,
		ccs:          ccs,
		draft:        draft,
		edit:         edit,
		repo:         repo,
		reviewBranch: reviewBranch,
		reviewers:    reviewers,
	}
}

var conflictMessage = `
######################################################################
Your branch and the project master branch contain conflicting changes.
Please run 'git pull origin master', resolve all conflicts, and then
run 'git veyron review' again.
######################################################################
`

var noCommitsMessage = `
######################################################################
Your branch has no new changes. Please make some changes and commit
them before running 'git veyron review' again.
######################################################################
`

var noChangeIdMessage = `
######################################################################
Missing Change-Id.  Please run ./scripts/setup/repo/init.sh and then
run 'git veyron review' again.
######################################################################
`

var defaultMessageHeader = `
PLEASE EDIT THIS MESSAGE!

# You are about to submit the following commits for review:
#
`

// Change-Ids start with 'I' and are followed by 40 characters of hex.
var reChangeId *regexp.Regexp = regexp.MustCompile("Change-Id: I[0123456789abcdefABCDEF]{40}")

// defaultCommitMessage creates the default commit message from the list of
// commits on the branch.
func (r *review) defaultCommitMessage() (string, error) {
	commitMessages, err := git.CommitMessages(r.branch, r.reviewBranch)
	if err != nil {
		return "", fmt.Errorf("git.CommitMessages(%v, %v) failed: %v", r.branch, r.reviewBranch, err)
	}
	// Strip "Change-Id: ..." from the commit messages.
	strippedMessages := reChangeId.ReplaceAllLiteralString(commitMessages, "")
	// Add comment markers (#) to every line.
	commentedMessages := "# " + strings.Replace(strippedMessages, "\n", "\n# ", -1)
	message := defaultMessageHeader + commentedMessages
	return message, nil
}

// createReviewBranch creates a clean review branch from master and
// squashes the commits into one, with the supplied message.
func (r *review) createReviewBranch(message string) error {
	fmt.Println("### Creating a temporary review branch. ###")
	if err := git.Fetch(); err != nil {
		return fmt.Errorf("git.Fetch() failed: %v", err)
	}
	_ = git.ForceDeleteBranch(r.reviewBranch)
	upstream := "origin/master"
	if err := git.CreateBranchWithUpstream(r.reviewBranch, upstream); err != nil {
		return fmt.Errorf("git.CreateBranch(%v, %v) failed: %v", r.reviewBranch, upstream, err)
	}
	{
		hasDiff, err := git.BranchesDiffer(r.branch, r.reviewBranch)
		if err != nil {
			return fmt.Errorf("git.BranchesDiffer(%v, %v) failed: %v", r.branch, r.reviewBranch, err)
		}
		if !hasDiff {
			fmt.Printf("%s", noCommitsMessage)
			return EmptyChangeError(r.branch)
		}
	}
	// If message is empty, replace it with the default.
	if len(message) == 0 {
		var err error
		message, err = r.defaultCommitMessage()
		if err != nil {
			return fmt.Errorf("defaultCommitMessage() failed: %v", err)
		}
	}
	fmt.Printf("### Squashing commits into the review branch. ###\n")
	s := git.NewSquasher(r.edit)
	if err := s.Squash(r.branch, r.reviewBranch, message); err != nil {
		fmt.Printf("%s", conflictMessage)
		return fmt.Errorf("git.SquashInto(%v,%v,%v) failed: %v", r.branch, r.reviewBranch, message, err)
	}
	return nil
}

// ensureChangeId makes sure that the last commit contains a Change-Id, and
// returns an error if it does not.
func (r *review) ensureChangeId() error {
	latestCommitMessage, err := git.LatestCommitMessage()
	if err != nil {
		return fmt.Errorf("git.LatestCommitMessage() failed: %v", err)
	}
	changeId := reChangeId.FindString(latestCommitMessage)
	if changeId == "" {
		fmt.Printf("%s", noChangeIdMessage)
		return NoChangeIdError(latestCommitMessage)
	}
	return nil
}

// cleanup cleans up after the review.
func (r *review) cleanup(stash bool) {
	fmt.Println("### Cleaning up. ###")
	if err := git.CheckoutBranch(r.branch); err != nil {
		fmt.Println("git.CheckoutBranch(%v) failed: %v", r.branch, err)
	}
	_ = git.ForceDeleteBranch(r.reviewBranch)
	if stash {
		if err := git.StashPop(); err != nil {
			fmt.Println("git.StashPop() failed: %v", err)
		}
	}
}

// run implements the end-to-end functionality of the review command.
func (r *review) run() error {
	fmt.Printf("Branch name: %s\n", r.branch)
	if r.branch == "master" {
		fmt.Errorf("Cannot do a review from the 'master' branch.")
		return errOperationFailed
	}
	filename, err := getCommitMessageFilename()
	if err != nil {
		fmt.Errorf("%v", err)
		return errOperationFailed
	}
	stash, err := stashUncommittedChanges()
	if err != nil {
		fmt.Errorf("%v", err)
		return errOperationFailed
	}
	defer r.cleanup(stash)
	if err := r.createReviewBranch(readFile(filename)); err != nil {
		fmt.Errorf("%v", err)
		return errOperationFailed
	}
	if err := r.updateReviewMessage(filename); err != nil {
		fmt.Errorf("%v", err)
		return errOperationFailed
	}
	if err := r.send(); err != nil {
		fmt.Errorf("%v", err)
		return errOperationFailed
	}
	fmt.Println("### Success. ###")
	return nil
}

// send sends the current branch out for review.
func (r *review) send() error {
	if err := r.ensureChangeId(); err != nil {
		return err
	}
	fmt.Println("### Sending review to Gerrit. ###")
	if err := git.GerritReview(r.repo, r.draft, r.reviewers, r.ccs); err != nil {
		fmt.Printf("%s", conflictMessage)
		return fmt.Errorf("git.GerritReview(%v, %v, %v, %v) failed: %v",
			r.repo, r.draft, r.reviewers, r.ccs, err)
	}
	return nil
}

// updateReviewMessage writes the commit message to the specified
// file. It then adds that file to the original branch, and makes sure
// it is not on the review branch.
func (r *review) updateReviewMessage(filename string) error {
	fmt.Printf("### Updating review commit message. ###\n")
	if err := git.CheckoutBranch(r.reviewBranch); err != nil {
		return fmt.Errorf("git.CheckoutBranch(%v) failed: %v", r.reviewBranch, err)
	}
	newMessage, err := git.LatestCommitMessage()
	if err != nil {
		return fmt.Errorf("git.LatestCommitMessage() failed: %v", err)
	}
	if err := git.CheckoutBranch(r.branch); err != nil {
		return fmt.Errorf("git.CheckoutBranch(%v) failed: %v", r.branch, err)
	}
	if err := writeFile(filename, newMessage); err != nil {
		return fmt.Errorf("writeFile(%v, %v) failed: %v", filename, newMessage, err)
	}
	if err := git.CommitFile(filename, "Update gerrit commit message."); err != nil {
		return fmt.Errorf("git.CommitFile(%v) failed: %v", filename, err)
	}
	// Delete the commit message from review branch.
	if err := git.CheckoutBranch(r.reviewBranch); err != nil {
		return fmt.Errorf("git.CheckoutBranch(%v) failed: %v", r.reviewBranch, err)
	}
	if fileExists(filename) {
		if err := git.Remove(filename); err != nil {
			return fmt.Errorf("git.Remove(%v) failed: %v", filename, err)
		}
		if err := git.CommitAmend(newMessage); err != nil {
			return fmt.Errorf("git.CommitAmend(%v) failed: %v", newMessage, err)
		}
	}
	return nil
}

// fileExists returns true iff the file exists.
func fileExists(filename string) bool {
	if _, err := os.Stat(filename); err == nil {
		return true
	}
	return false
}

// getCommitMessageFilename returns the name of the file that will get
// used for the Gerrit commit message.
func getCommitMessageFilename() (string, error) {
	topLevel, err := git.TopLevel()
	if err != nil {
		return "", fmt.Errorf("git.TopLevel() failed: %v", err)
	}
	return filepath.Join(topLevel, ".gerrit_commit_message"), nil
}

// readFile returns the data in a file as a string. Returns empty
// string if the file does not exist.
func readFile(filename string) string {
	if fileExists(filename) {
		contents, _ := ioutil.ReadFile(filename)
		return string(contents)
	}
	return ""
}

// stashUncommittedChanges stashes any work in progress and returns a
// flag that indicates whether anything has been stashed.
func stashUncommittedChanges() (bool, error) {
	oldSize, err := git.StashSize()
	if err != nil {
		return false, fmt.Errorf("git.StashSize() failed: %v", err)
	}
	if err := git.Stash(); err != nil {
		return false, fmt.Errorf("git.Stash() failed: %v", err)
	}
	newSize, err := git.StashSize()
	if err != nil {
		return false, fmt.Errorf("git.StashSize() failed: %v", err)
	}
	return newSize > oldSize, nil
}

// writeFile writes the message string to the file.
func writeFile(filename, message string) error {
	return ioutil.WriteFile(filename, []byte(message), 0644)
}

// writeFileExecutable writes the message string to the file and makes it executable.
func writeFileExecutable(filename, message string) error {
	return ioutil.WriteFile(filename, []byte(message), 0777)
}
