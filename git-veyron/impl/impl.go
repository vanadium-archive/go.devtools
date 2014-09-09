package impl

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"tools/lib/cmd"
	"tools/lib/cmdline"
	gitlib "tools/lib/git"
)

const (
	ROOT_ENV = "VEYRON_ROOT"
)

var (
	ccsFlag       string
	draftFlag     bool
	forceFlag     bool
	reviewersFlag string
	verboseFlag   bool
)

var (
	root = func() string {
		result := os.Getenv(ROOT_ENV)
		if result == "" {
			panic(fmt.Sprintf("%v is not set", ROOT_ENV))
		}
		return result
	}()
)

// init carries out the package initialization.
func init() {
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdCleanup.Flags.BoolVar(&forceFlag, "f", false, "Ignore unmerged changes.")
	cmdReview.Flags.BoolVar(&draftFlag, "d", false, "Send draft change list.")
	cmdReview.Flags.StringVar(&reviewersFlag, "r", "", "Comma-seperated list of emails or LDAPs to request review.")
	cmdReview.Flags.StringVar(&ccsFlag, "cc", "", "Comma-seperated list of emails or LDAPs to cc.")
}

var cmdRoot = &cmdline.Command{
	Name:  "git veyron",
	Short: "Command-line tool for interacting with the Veyron Gerrit server",
	Long: `
The veyron tool facilitates interaction with the Veyron Gerrit server.
In particular, it can be used to export changes from a local branch
to the Gerrit server.
`,
	Children: []*cmdline.Command{cmdCleanup, cmdReview, cmdSelfUpdate, cmdVersion},
}

// Root returns a command that represents the root of the git veyron tool.
func Root() *cmdline.Command {
	return cmdRoot
}

// cmmCleanup represent the 'cleanup' command of the git veyron tool.
var cmdCleanup = &cmdline.Command{
	Run:   runCleanup,
	Name:  "cleanup",
	Short: "Clean up branches that have been merged",
	Long: `
The cleanup command checks that the given branches have been merged
into the master branch. If a branch differs from the master, it
reports the difference and stops. Otherwise, it deletes the branch.
`,
	ArgsName: "<branches>",
	ArgsLong: "<branches> is a list of branches to cleanup.",
}

func cleanup(command *cmdline.Command, git *gitlib.Git, branches []string) error {
	if len(branches) == 0 {
		return command.Errorf("cleanup requires at least one argument")
	}
	currentBranch, err := git.CurrentBranchName()
	if err != nil {
		return err
	}
	stashed, err := git.Stash()
	if err != nil {
		return err
	}
	if stashed {
		defer git.StashPop()
	}
	if err := git.CheckoutBranch("master"); err != nil {
		return err
	}
	defer git.CheckoutBranch(currentBranch)
	if err := git.Pull("origin", "master"); err != nil {
		return err
	}
	for _, branch := range branches {
		cleanupFn := func() error { return cleanupBranch(git, branch) }
		if err := cmd.Log(fmt.Sprintf("Cleaning up branch %q", branch), cleanupFn); err != nil {
			return err
		}
	}
	return nil
}

func cleanupBranch(git *gitlib.Git, branch string) error {
	if err := git.CheckoutBranch(branch); err != nil {
		return err
	}
	if !forceFlag {
		if err := git.Merge("master", false); err != nil {
			return err
		}
		files, err := git.ModifiedFiles("master", branch)
		if err != nil {
			return err
		}
		// A feature branch is considered merged with
		// the master, when there are no differences
		// or the only difference is the gerrit commit
		// message file.
		if len(files) != 0 && (len(files) != 1 || files[0] != ".gerrit_commit_message") {
			return fmt.Errorf("unmerged changes in\n%s", strings.Join(files, "\n"))
		}
	}
	if err := git.CheckoutBranch("master"); err != nil {
		return err
	}
	if err := git.ForceDeleteBranch(branch); err != nil {
		return err
	}
	reviewBranch := branch + "-REVIEW"
	if git.BranchExists(reviewBranch) {
		if err := git.ForceDeleteBranch(reviewBranch); err != nil {
			return err
		}
	}
	return nil
}

func runCleanup(command *cmdline.Command, args []string) error {
	return cleanup(command, gitlib.New(verboseFlag), args)
}

// cmdReview represent the 'review' command of the git veyron tool.
var cmdReview = &cmdline.Command{
	Run:   runReview,
	Name:  "review",
	Short: "Send changes from a local branch to Gerrit for review",
	Long: `
Squashes all commits of a local branch into a single commit and
submits that commit to Gerrit as a single change list. You can run
it multiple times to send more patch sets to the change list.
`,
}

type ChangeConflictError string

func (s ChangeConflictError) Error() string {
	result := "change conflicts with the remote master branch\n\n"
	result += "To resolve this problem, run 'git pull origin master',\n"
	result += "resolve the conflicts identified below, and then try again.\n"
	result += string(s)
	return result
}

type EmptyChangeError struct{}

func (_ EmptyChangeError) Error() string {
	return "current branch has no commits"
}

type GerritError string

func (s GerritError) Error() string {
	result := "sending code review failed\n\n"
	result += string(s)
	return result
}

type GoFormatError []string

func (s GoFormatError) Error() string {
	result := "change does not adhere to the Go formatting conventions\n\n"
	result += "To resolve this problem, run 'go fmt' for the following file(s):\n"
	result += "  " + strings.Join(s, "\n  ")
	return result
}

type NoChangeIDError struct{}

func (_ NoChangeIDError) Error() string {
	result := "change is missing a Change-ID"
	return result
}

var defaultMessageHeader = `
# Describe your change, specifying what package(s) your change pertains to,
# followed by a short summary and, in case of non-trivial changes, a detailed
# description.
#
# For example:
#
# veyron/runtimes/google/ipc/stream/proxy: add publish address
#
# The listen address is not always the same at the address that external
# users need to connect to. This change adds a new argument to proxy.New()
# to specify the published address that clients should connect to.

# FYI, you are about to submit the following local commits for review:
#
`

// runReview is a wrapper that sets up and runs a review instance.
func runReview(*cmdline.Command, []string) error {
	git := gitlib.New(verboseFlag)
	branch, err := git.CurrentBranchName()
	if err != nil {
		return err
	}
	edit, repo := true, ""
	r := NewReview(draftFlag, edit, branch, repo, reviewersFlag, ccsFlag)
	return r.run(git)
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

// Change-Ids start with 'I' and are followed by 40 characters of hex.
var reChangeID *regexp.Regexp = regexp.MustCompile("Change-Id: I[0123456789abcdefABCDEF]{40}")

// findGoBinary returns the path to the given Go binary.
func findGoBinary(name string) (string, error) {
	envbin := filepath.Join(root, "environment", "go", runtime.GOOS, runtime.GOARCH, "go", "bin", name)
	if _, err := os.Stat(envbin); err == nil {
		return envbin, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("Stat(%v) failed: %v", envbin, err)
	}
	pathbin, err := exec.LookPath(name)
	switch {
	case err == nil:
		return pathbin, nil
	case err == exec.ErrNotFound:
		return "", fmt.Errorf("%q does not exist and %q not found in PATH", envbin, name)
	default:
		return "", fmt.Errorf("LookPath(%q) failed: %v", name, err)
	}
}

// checkGoFormat checks if the code to be submitted needs to be
// formatted with "go fmt".
func (r *review) checkGoFormat(git *gitlib.Git) error {
	if err := git.Fetch(); err != nil {
		return err
	}
	files, err := git.ModifiedFiles("FETCH_HEAD", r.branch)
	if err != nil {
		return err
	}
	gofmt, err := findGoBinary("gofmt")
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	topLevel, err := git.TopLevel()
	if err != nil {
		return err
	}
	os.Chdir(topLevel)
	ill := make([]string, 0)
	for _, file := range files {
		path := filepath.Join(topLevel, file)
		if strings.HasSuffix(file, ".go") {
			// Skip files deleted by the change.
			if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
				continue
			}
			// Check if the formatting of <file> differs
			// from gofmt.
			out, _, err := cmd.RunOutput(verboseFlag, gofmt, "-l", path)
			if err != nil || len(out) != 0 {
				ill = append(ill, file)
			}
		}
	}
	if len(ill) != 0 {
		return GoFormatError(ill)
	}
	return nil
}

// cleanup cleans up after the review.
func (r *review) cleanup(stashed bool, git *gitlib.Git) {
	if err := git.CheckoutBranch(r.branch); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
	if git.BranchExists(r.reviewBranch) {
		if err := git.ForceDeleteBranch(r.reviewBranch); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}
	if stashed {
		if err := git.StashPop(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}
}

// createReviewBranch creates a clean review branch from master and
// squashes the commits into one, with the supplied message.
func (r *review) createReviewBranch(message string, git *gitlib.Git) error {
	if err := git.Fetch(); err != nil {
		return err
	}
	if git.BranchExists(r.reviewBranch) {
		if err := git.ForceDeleteBranch(r.reviewBranch); err != nil {
			return err
		}
	}
	upstream := "origin/master"
	if err := git.CreateBranchWithUpstream(r.reviewBranch, upstream); err != nil {
		return err
	}
	{
		hasDiff, err := git.BranchesDiffer(r.branch, r.reviewBranch)
		if err != nil {
			return err
		}
		if !hasDiff {
			return EmptyChangeError(struct{}{})
		}
	}
	// If message is empty, replace it with the default.
	if len(message) == 0 {
		var err error
		message, err = r.defaultCommitMessage(git)
		if err != nil {
			return err
		}
	}
	if err := git.CheckoutBranch(r.reviewBranch); err != nil {
		return err
	}
	if err := git.Merge(r.branch, true); err != nil {
		return ChangeConflictError(err.Error())
	}
	c := git.NewCommitter(r.edit)
	if err := c.Commit(message); err != nil {
		return err
	}
	return nil
}

// defaultCommitMessage creates the default commit message from the list of
// commits on the branch.
func (r *review) defaultCommitMessage(git *gitlib.Git) (string, error) {
	commitMessages, err := git.CommitMessages(r.branch, r.reviewBranch)
	if err != nil {
		return "", err
	}
	// Strip "Change-Id: ..." from the commit messages.
	strippedMessages := reChangeID.ReplaceAllLiteralString(commitMessages, "")
	// Add comment markers (#) to every line.
	commentedMessages := "# " + strings.Replace(strippedMessages, "\n", "\n# ", -1)
	message := defaultMessageHeader + commentedMessages
	return message, nil
}

// ensureChangeID makes sure that the last commit contains a Change-Id, and
// returns an error if it does not.
func (r *review) ensureChangeID(git *gitlib.Git) error {
	latestCommitMessage, err := git.LatestCommitMessage()
	if err != nil {
		return err
	}
	changeID := reChangeID.FindString(latestCommitMessage)
	if changeID == "" {
		return NoChangeIDError(struct{}{})
	}
	return nil
}

// run implements the end-to-end functionality of the review command.
func (r *review) run(git *gitlib.Git) error {
	if err := r.checkGoFormat(git); err != nil {
		return err
	}
	if r.branch == "master" {
		return fmt.Errorf("cannot do a review from the 'master' branch.")
	}
	filename, err := getCommitMessageFilename(git)
	if err != nil {
		return err
	}
	stashed, err := git.Stash()
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	topLevel, err := git.TopLevel()
	if err != nil {
		return err
	}
	os.Chdir(topLevel)
	defer r.cleanup(stashed, git)
	if err := r.createReviewBranch(readFile(filename), git); err != nil {
		return err
	}
	if err := r.updateReviewMessage(filename, git); err != nil {
		return err
	}
	if err := r.send(git); err != nil {
		return err
	}
	return nil
}

// send sends the current branch out for review.
func (r *review) send(git *gitlib.Git) error {
	if err := r.ensureChangeID(git); err != nil {
		return err
	}
	if err := git.GerritReview(r.repo, r.draft, r.reviewers, r.ccs, r.branch); err != nil {
		return GerritError(err.Error())
	}
	return nil
}

// updateReviewMessage writes the commit message to the specified
// file. It then adds that file to the original branch, and makes sure
// it is not on the review branch.
func (r *review) updateReviewMessage(filename string, git *gitlib.Git) error {
	if err := git.CheckoutBranch(r.reviewBranch); err != nil {
		return err
	}
	newMessage, err := git.LatestCommitMessage()
	if err != nil {
		return err
	}
	if err := git.CheckoutBranch(r.branch); err != nil {
		return err
	}
	if err := writeFile(filename, newMessage); err != nil {
		return fmt.Errorf("writeFile(%v, %v) failed: %v", filename, newMessage, err)
	}
	if err := git.CommitFile(filename, "Update gerrit commit message."); err != nil {
		return err
	}
	// Delete the commit message from review branch.
	if err := git.CheckoutBranch(r.reviewBranch); err != nil {
		return err
	}
	if fileExists(filename) {
		if err := git.Remove(filename); err != nil {
			return err
		}
		if err := git.CommitAmend(newMessage); err != nil {
			return err
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
func getCommitMessageFilename(git *gitlib.Git) (string, error) {
	topLevel, err := git.TopLevel()
	if err != nil {
		return "", err
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

// writeFile writes the message string to the file.
func writeFile(filename, message string) error {
	return ioutil.WriteFile(filename, []byte(message), 0644)
}

// writeFileExecutable writes the message string to the file and makes it executable.
func writeFileExecutable(filename, message string) error {
	return ioutil.WriteFile(filename, []byte(message), 0777)
}

// cmdSelfUpdate represents the 'selfupdate' command of the veyron
// tool.
var cmdSelfUpdate = &cmdline.Command{
	Run:   runSelfUpdate,
	Name:  "selfupdate",
	Short: "Update the veyron tool",
	Long:  "Download and install the latest version of the veyron tool.",
}

func runSelfUpdate(command *cmdline.Command, args []string) error {
	git := gitlib.New(verboseFlag)
	tool := "git-veyron"
	return cmd.Log(fmt.Sprintf("Updating tool %q", tool), func() error { return git.SelfUpdate(tool) })
}

// cmdVersion represent the 'version' command of the review tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the git veyron tool.",
}

const version string = "1.1.0"

// commitId should be over-written during build:
// go build -ldflags "-X tools/git-veyron/impl.commitId <commitId>" tools/git-veyron
var commitId string = "test-build"

func runVersion(cmd *cmdline.Command, args []string) error {
	fmt.Printf("git veyron tool version %v (build %v)\n", version, commitId)
	return nil
}
