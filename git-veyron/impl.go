package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/gerrit"
	"tools/lib/gitutil"
	"tools/lib/runutil"
	"tools/lib/util"
	"tools/lib/version"
)

var (
	ccsFlag         string
	currentFlag     bool
	draftFlag       bool
	forceFlag       bool
	gofmtFlag       bool
	masterFlag      bool
	reviewersFlag   string
	verboseFlag     bool
	uncommittedFlag bool
	untrackedFlag   bool
)

// init carries out the package initialization.
func init() {
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdCleanup.Flags.BoolVar(&forceFlag, "f", false, "Ignore unmerged changes.")
	cmdReview.Flags.BoolVar(&draftFlag, "d", false, "Send a draft changelist.")
	cmdReview.Flags.StringVar(&reviewersFlag, "r", "", "Comma-seperated list of emails or LDAPs to request review.")
	cmdReview.Flags.StringVar(&ccsFlag, "cc", "", "Comma-seperated list of emails or LDAPs to cc.")
	cmdReview.Flags.BoolVar(&uncommittedFlag, "check-uncommitted", true, "Check that no uncommitted changes exist.")
	cmdReview.Flags.BoolVar(&gofmtFlag, "check-gofmt", true, "Check that no go fmt violations exist.")
	cmdStatus.Flags.BoolVar(&masterFlag, "show-master", false, "Show master branches in the status.")
	cmdStatus.Flags.BoolVar(&uncommittedFlag, "show-uncommitted", true, "Indicate if there are any uncommitted changes.")
	cmdStatus.Flags.BoolVar(&untrackedFlag, "show-untracked", true, "Indicate if there are any untracked files.")
	cmdStatus.Flags.BoolVar(&currentFlag, "show-current", false, "Show the name of the current repo.")
}

var cmdRoot = &cmdline.Command{
	Name:  "git-veyron",
	Short: "Tool for interacting with the Veyron Gerrit server",
	Long: `
The git-veyron tool facilitates interaction with the Veyron Gerrit server.
In particular, it can be used to export changelists from a local branch
to the Gerrit server.
`,
	Children: []*cmdline.Command{cmdCleanup, cmdReview, cmdStatus, cmdVersion},
}

// root returns a command that represents the root of the git-veyron tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmmCleanup represents the 'cleanup' command of the git-veyron tool.
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

func cleanup(command *cmdline.Command, git *gitutil.Git, run *runutil.Run, branches []string) error {
	if len(branches) == 0 {
		return command.UsageErrorf("cleanup requires at least one argument")
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
	if err := git.CheckoutBranch("master", !gitutil.Force); err != nil {
		return err
	}
	defer git.CheckoutBranch(currentBranch, !gitutil.Force)
	if err := git.Pull("origin", "master"); err != nil {
		return err
	}
	for _, branch := range branches {
		cleanupFn := func() error { return cleanupBranch(git, branch) }
		if err := run.Function(cleanupFn, "Cleaning up branch %q", branch); err != nil {
			return err
		}
	}
	return nil
}

func cleanupBranch(git *gitutil.Git, branch string) error {
	if err := git.CheckoutBranch(branch, !gitutil.Force); err != nil {
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
	if err := git.CheckoutBranch("master", !gitutil.Force); err != nil {
		return err
	}
	if err := git.DeleteBranch(branch, gitutil.Force); err != nil {
		return err
	}
	reviewBranch := branch + "-REVIEW"
	if git.BranchExists(reviewBranch) {
		if err := git.DeleteBranch(reviewBranch, gitutil.Force); err != nil {
			return err
		}
	}
	return nil
}

func runCleanup(command *cmdline.Command, args []string) error {
	ctx := util.NewContext(verboseFlag, command.Stdout(), command.Stderr())
	return cleanup(command, ctx.Git(), ctx.Run(), args)
}

// cmdReview represents the 'review' command of the git-veyron tool.
var cmdReview = &cmdline.Command{
	Run:   runReview,
	Name:  "review",
	Short: "Send a changelist from a local branch to Gerrit for review",
	Long: `
Squashes all commits of a local branch into a single "changelist" and
sends this changelist to Gerrit as a single commit. First time the
command is invoked, it generates a Change-Id for the changelist, which
is appended to the commit message. Consecutive invocations of the
command use the same Change-Id by default, informing Gerrit that the
incomming commit is an update of an existing changelist.
`,
}

type changeConflictError string

func (s changeConflictError) Error() string {
	result := "changelist conflicts with the remote master branch\n\n"
	result += "To resolve this problem, run 'git pull origin master',\n"
	result += "resolve the conflicts identified below, and then try again.\n"
	result += string(s)
	return result
}

type emptyChangeError struct{}

func (_ emptyChangeError) Error() string {
	return "current branch has no commits"
}

type gerritError string

func (s gerritError) Error() string {
	result := "sending code review failed\n\n"
	result += string(s)
	return result
}

type goFormatError []string

func (s goFormatError) Error() string {
	result := "changelist does not adhere to the Go formatting conventions\n\n"
	result += "To resolve this problem, run 'go fmt' for the following file(s):\n"
	result += "  " + strings.Join(s, "\n  ")
	return result
}

type noChangeIDError struct{}

func (_ noChangeIDError) Error() string {
	result := "changelist is missing a Change-ID"
	return result
}

type uncommittedChangesError []string

func (s uncommittedChangesError) Error() string {
	result := "uncommitted local changes in files:\n"
	result += "  " + strings.Join(s, "\n  ")
	return result
}

var defaultMessageHeader = `
# Describe your changelist, specifying what package(s) your change
# pertains to, followed by a short summary and, in case of non-trivial
# changelists, provide a detailed description.
#
# For example:
#
# veyron/runtimes/google/ipc/stream/proxy: add publish address
#
# The listen address is not always the same as the address that external
# users need to connect to. This CL adds a new argument to proxy.New()
# to specify the published address that clients should connect to.

# FYI, you are about to submit the following local commits for review:
#
`

// runReview is a wrapper that sets up and runs a review instance.
func runReview(command *cmdline.Command, _ []string) error {
	ctx, edit, repo := util.NewContext(verboseFlag, command.Stdout(), command.Stderr()), true, ""
	review, err := NewReview(ctx, draftFlag, edit, repo, reviewersFlag, ccsFlag)
	if err != nil {
		return err
	}
	return review.run()
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
	// git is an instance of a git command executor.
	git *gitutil.Git
	// runner is an instance of a general purpose command executor.
	runner *runutil.Run
	// repo is the name of the gerrit repository.
	repo string
	// reviewBranch is the name of the temporary git branch used to send the review.
	reviewBranch string
	// reviewers is the list of LDAPs or emails to request a review from.
	reviewers string
}

// NewReview is the review factory.
func NewReview(ctx *util.Context, draft, edit bool, repo, reviewers, ccs string) (*review, error) {
	branch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return nil, err
	}
	reviewBranch := branch + "-REVIEW"
	return &review{
		branch:       branch,
		ccs:          ccs,
		draft:        draft,
		edit:         edit,
		git:          ctx.Git(),
		runner:       ctx.Run(),
		repo:         repo,
		reviewBranch: reviewBranch,
		reviewers:    reviewers,
	}, nil
}

// Change-Ids start with 'I' and are followed by 40 characters of hex.
var reChangeID *regexp.Regexp = regexp.MustCompile("Change-Id: I[0123456789abcdefABCDEF]{40}")

// checkGoFormat checks if the code to be submitted needs to be
// formatted with "go fmt".
func (r *review) checkGoFormat() error {
	if err := r.git.Fetch("origin", "master"); err != nil {
		return err
	}
	files, err := r.git.ModifiedFiles("FETCH_HEAD", r.branch)
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	topLevel, err := r.git.TopLevel()
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
			fmtCmd := exec.Command("veyron", "go", "fmt", path)
			out, err := fmtCmd.Output()
			if err != nil {
				return fmt.Errorf("failed to check Go format: %v\n%v\n%s", err, strings.Join(fmtCmd.Args, " "), out)
			}
			if len(out) != 0 {
				ill = append(ill, file)
			}
		}
	}
	if len(ill) != 0 {
		return goFormatError(ill)
	}
	return nil
}

// cleanup cleans up after the review.
func (r *review) cleanup(stashed bool) {
	if err := r.git.CheckoutBranch(r.branch, !gitutil.Force); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
	if r.git.BranchExists(r.reviewBranch) {
		if err := r.git.DeleteBranch(r.reviewBranch, gitutil.Force); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}
	if stashed {
		if err := r.git.StashPop(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}
}

// createReviewBranch creates a clean review branch from master and
// squashes the commits into one, with the supplied message.
func (r *review) createReviewBranch(message string) error {
	if err := r.git.Fetch("origin", "master"); err != nil {
		return err
	}
	if r.git.BranchExists(r.reviewBranch) {
		if err := r.git.DeleteBranch(r.reviewBranch, gitutil.Force); err != nil {
			return err
		}
	}
	upstream := "origin/master"
	if err := r.git.CreateBranchWithUpstream(r.reviewBranch, upstream); err != nil {
		return err
	}
	{
		hasDiff, err := r.git.BranchesDiffer(r.branch, r.reviewBranch)
		if err != nil {
			return err
		}
		if !hasDiff {
			return emptyChangeError(struct{}{})
		}
	}
	// If message is empty, replace it with the default.
	if len(message) == 0 {
		var err error
		message, err = r.defaultCommitMessage()
		if err != nil {
			return err
		}
	}
	if err := r.git.CheckoutBranch(r.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	if err := r.git.Merge(r.branch, true); err != nil {
		return changeConflictError(err.Error())
	}
	c := r.git.NewCommitter(r.edit)
	if err := c.Commit(message); err != nil {
		return err
	}
	return nil
}

// defaultCommitMessage creates the default commit message from the list of
// commits on the branch.
func (r *review) defaultCommitMessage() (string, error) {
	commitMessages, err := r.git.CommitMessages(r.branch, r.reviewBranch)
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
func (r *review) ensureChangeID() error {
	latestCommitMessage, err := r.git.LatestCommitMessage()
	if err != nil {
		return err
	}
	changeID := reChangeID.FindString(latestCommitMessage)
	if changeID == "" {
		return noChangeIDError(struct{}{})
	}
	return nil
}

// run implements the end-to-end functionality of the review command.
func (r *review) run() error {
	if uncommittedFlag {
		changes, err := r.git.FilesWithUncommittedChanges()
		if err != nil {
			return err
		}
		if len(changes) != 0 {
			return uncommittedChangesError(changes)
		}
	}
	if gofmtFlag {
		if err := r.checkGoFormat(); err != nil {
			return err
		}
	}
	if r.branch == "master" {
		return fmt.Errorf("cannot do a review from the 'master' branch.")
	}
	filename, err := r.getCommitMessageFilename()
	if err != nil {
		return err
	}
	stashed, err := r.git.Stash()
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	topLevel, err := r.git.TopLevel()
	if err != nil {
		return err
	}
	os.Chdir(topLevel)
	defer r.cleanup(stashed)
	if err := r.createReviewBranch(readFile(filename)); err != nil {
		return err
	}
	if err := r.updateReviewMessage(filename); err != nil {
		return err
	}
	if err := r.send(); err != nil {
		return err
	}
	return nil
}

// send sends the current branch out for review.
func (r *review) send() error {
	if err := r.ensureChangeID(); err != nil {
		return err
	}
	if err := gerrit.Review(r.runner, r.repo, r.draft, r.reviewers, r.ccs, r.branch); err != nil {
		return gerritError(err.Error())
	}
	return nil
}

// updateReviewMessage writes the commit message to the specified
// file. It then adds that file to the original branch, and makes sure
// it is not on the review branch.
func (r *review) updateReviewMessage(filename string) error {
	if err := r.git.CheckoutBranch(r.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	newMessage, err := r.git.LatestCommitMessage()
	if err != nil {
		return err
	}
	if err := r.git.CheckoutBranch(r.branch, !gitutil.Force); err != nil {
		return err
	}
	if err := writeFile(filename, newMessage); err != nil {
		return fmt.Errorf("writeFile(%v, %v) failed: %v", filename, newMessage, err)
	}
	if err := r.git.CommitFile(filename, "Update gerrit commit message."); err != nil {
		return err
	}
	// Delete the commit message from review branch.
	if err := r.git.CheckoutBranch(r.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	if fileExists(filename) {
		if err := r.git.Remove(filename); err != nil {
			return err
		}
		if err := r.git.CommitAmend(newMessage); err != nil {
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
func (r *review) getCommitMessageFilename() (string, error) {
	topLevel, err := r.git.TopLevel()
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

// cmdStatus represents the 'status' command of the git-veyron tool.
var cmdStatus = &cmdline.Command{
	Run:   runStatus,
	Name:  "status",
	Short: "Print a succint status of the veyron repositories",
	Long: `
Reports current branches of existing veyron repositories as well as an
indication of the status:
  *  indicates whether a repository contains uncommitted changes
  %  indicates whether a repository contains untracked files
`,
}

func runStatus(command *cmdline.Command, args []string) error {
	ctx := util.NewContext(verboseFlag, command.Stdout(), command.Stderr())
	projects, err := util.LocalProjects(ctx)
	if err != nil {
		return err
	}
	names := []string{}
	for name, project := range projects {
		if project.Protocol == "git" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	// Get the name of the current repository, if applicable.
	currentRepo, _ := ctx.Git().RepoName()
	var statuses []string
	for _, name := range names {
		if err := os.Chdir(projects[name].Path); err != nil {
			return fmt.Errorf("Chdir(%v) failed: %v", projects[name].Path, err)
		}
		branch, err := ctx.Git().CurrentBranchName()
		if err != nil {
			return err
		}
		status := ""
		if uncommittedFlag {
			uncommitted, err := ctx.Git().HasUncommittedChanges()
			if err != nil {
				return err
			}
			if uncommitted {
				status += "*"
			}
		}
		if untrackedFlag {
			untracked, err := ctx.Git().HasUntrackedFiles()
			if err != nil {
				return err
			}
			if untracked {
				status += "%"
			}
		}
		short := branch + status
		long := filepath.Base(name) + ":" + short
		if currentRepo == name {
			if currentFlag {
				statuses = append([]string{long}, statuses...)
			} else {
				statuses = append([]string{short}, statuses...)
			}
		} else {
			if masterFlag || branch != "master" {
				statuses = append(statuses, long)
			}
		}
	}
	fmt.Println(strings.Join(statuses, ","))
	return nil
}

// cmdVersion represents the 'version' command of the git-veyron tool.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the git-veyron tool.",
}

func runVersion(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "git-veyron tool version %v\n", version.Version)
	return nil
}
