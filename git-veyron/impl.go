package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"veyron.io/lib/cmdline"
	"veyron.io/tools/lib/gerrit"
	"veyron.io/tools/lib/gitutil"
	"veyron.io/tools/lib/util"
	"veyron.io/tools/lib/version"
)

const commitMessageFile = ".gerrit_commit_message"

var (
	ccsFlag         string
	currentFlag     bool
	draftFlag       bool
	dryRunFlag      bool
	forceFlag       bool
	depcopFlag      bool
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
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	cmdCleanup.Flags.BoolVar(&forceFlag, "f", false, "Ignore unmerged changes.")
	cmdReview.Flags.BoolVar(&draftFlag, "d", false, "Send a draft changelist.")
	cmdReview.Flags.StringVar(&reviewersFlag, "r", "", "Comma-seperated list of emails or LDAPs to request review.")
	cmdReview.Flags.StringVar(&ccsFlag, "cc", "", "Comma-seperated list of emails or LDAPs to cc.")
	cmdReview.Flags.BoolVar(&uncommittedFlag, "check_uncommitted", true, "Check that no uncommitted changes exist.")
	cmdReview.Flags.BoolVar(&depcopFlag, "check_depcop", true, "Check that no go-depcop violations exist.")
	cmdReview.Flags.BoolVar(&gofmtFlag, "check_gofmt", true, "Check that no go fmt violations exist.")
	cmdStatus.Flags.BoolVar(&masterFlag, "show_master", false, "Show master branches in the status.")
	cmdStatus.Flags.BoolVar(&uncommittedFlag, "show_uncommitted", true, "Indicate if there are any uncommitted changes.")
	cmdStatus.Flags.BoolVar(&untrackedFlag, "show_untracked", true, "Indicate if there are any untracked files.")
	cmdStatus.Flags.BoolVar(&currentFlag, "show_current", false, "Show the name of the current repo.")
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

func cleanup(ctx *util.Context, branches []string) error {
	currentBranch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return err
	}
	stashed, err := ctx.Git().Stash()
	if err != nil {
		return err
	}
	if stashed {
		defer ctx.Git().StashPop()
	}
	if err := ctx.Git().CheckoutBranch("master", !gitutil.Force); err != nil {
		return err
	}
	defer ctx.Git().CheckoutBranch(currentBranch, !gitutil.Force)
	if err := ctx.Git().Pull("origin", "master"); err != nil {
		return err
	}
	for _, branch := range branches {
		cleanupFn := func() error { return cleanupBranch(ctx, branch) }
		if err := ctx.Run().Function(cleanupFn, "Cleaning up branch %q", branch); err != nil {
			return err
		}
	}
	return nil
}

func cleanupBranch(ctx *util.Context, branch string) error {
	if err := ctx.Git().CheckoutBranch(branch, !gitutil.Force); err != nil {
		return err
	}
	if !forceFlag {
		if err := ctx.Git().Merge("master", false); err != nil {
			return err
		}
		files, err := ctx.Git().ModifiedFiles("master", branch)
		if err != nil {
			return err
		}
		// A feature branch is considered merged with
		// the master, when there are no differences
		// or the only difference is the gerrit commit
		// message file.
		if len(files) != 0 && (len(files) != 1 || files[0] != commitMessageFile) {
			return fmt.Errorf("unmerged changes in\n%s", strings.Join(files, "\n"))
		}
	}
	if err := ctx.Git().CheckoutBranch("master", !gitutil.Force); err != nil {
		return err
	}
	if err := ctx.Git().DeleteBranch(branch, gitutil.Force); err != nil {
		return err
	}
	reviewBranch := branch + "-REVIEW"
	if ctx.Git().BranchExists(reviewBranch) {
		if err := ctx.Git().DeleteBranch(reviewBranch, gitutil.Force); err != nil {
			return err
		}
	}
	return nil
}

func runCleanup(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		return command.UsageErrorf("cleanup requires at least one argument")
	}
	ctx := util.NewContextFromCommand(command, dryRunFlag, verboseFlag)
	return cleanup(ctx, args)
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
	result += "To resolve this problem, run 'veyron update; git merge master',\n"
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

type goDependencyError string

func (s goDependencyError) Error() string {
	result := "changelist introduces dependency violations\n\n"
	result += "To resolve this problem, fix the following violations:\n"
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
	ctx := util.NewContextFromCommand(command, dryRunFlag, verboseFlag)
	edit, repo := true, ""
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
	// ctx is an instance of the command-line tool context.
	ctx *util.Context
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
func NewReview(ctx *util.Context, draft, edit bool, repo, reviewers, ccs string) (*review, error) {
	branch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return nil, err
	}
	reviewBranch := branch + "-REVIEW"
	return &review{
		branch:       branch,
		ccs:          ccs,
		ctx:          ctx,
		draft:        draft,
		edit:         edit,
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
	if err := r.ctx.Git().Fetch("origin", "master"); err != nil {
		return err
	}
	files, err := r.ctx.Git().ModifiedFiles("FETCH_HEAD", r.branch)
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer r.ctx.Run().Chdir(wd)
	topLevel, err := r.ctx.Git().TopLevel()
	if err != nil {
		return err
	}
	if err := r.ctx.Run().Chdir(topLevel); err != nil {
		return err
	}
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
			var out bytes.Buffer
			opts := r.ctx.Run().Opts()
			opts.Stdout = &out
			opts.Stderr = &out
			if err := r.ctx.Run().CommandWithOpts(opts, "veyron", "go", "fmt", path); err != nil {
				return err
			}
			if out.Len() != 0 {
				ill = append(ill, file)
			}
		}
	}
	if len(ill) != 0 {
		return goFormatError(ill)
	}
	return nil
}

// checkDependencies checks if the code to be submitted meets the
// dependency constraints.
func (r *review) checkGoDependencies() error {
	var out bytes.Buffer
	opts := r.ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := r.ctx.Run().CommandWithOpts(opts, "veyron", "go", "list", "veyron.io/..."); err != nil {
		fmt.Println(out.String())
		return err
	}
	pkgs := strings.Split(strings.TrimSpace(out.String()), "\n")
	args := []string{"run", "go-depcop", "check"}
	args = append(args, pkgs...)
	out.Reset()
	if err := r.ctx.Run().CommandWithOpts(opts, "veyron", args...); err != nil {
		return goDependencyError(out.String())
	}
	return nil
}

// cleanup cleans up after the review.
func (r *review) cleanup(stashed bool) {
	if err := r.ctx.Git().CheckoutBranch(r.branch, !gitutil.Force); err != nil {
		fmt.Fprintf(r.ctx.Stderr(), "%v\n", err)
	}
	if r.ctx.Git().BranchExists(r.reviewBranch) {
		if err := r.ctx.Git().DeleteBranch(r.reviewBranch, gitutil.Force); err != nil {
			fmt.Fprintf(r.ctx.Stderr(), "%v\n", err)
		}
	}
	if stashed {
		if err := r.ctx.Git().StashPop(); err != nil {
			fmt.Fprintf(r.ctx.Stderr(), "%v\n", err)
		}
	}
}

// createReviewBranch creates a clean review branch from master and
// squashes the commits into one, with the supplied message.
func (r *review) createReviewBranch(message string) error {
	if err := r.ctx.Git().Fetch("origin", "master"); err != nil {
		return err
	}
	if r.ctx.Git().BranchExists(r.reviewBranch) {
		if err := r.ctx.Git().DeleteBranch(r.reviewBranch, gitutil.Force); err != nil {
			return err
		}
	}
	upstream := "origin/master"
	if err := r.ctx.Git().CreateBranchWithUpstream(r.reviewBranch, upstream); err != nil {
		return err
	}
	if !r.ctx.DryRun() {
		hasDiff, err := r.ctx.Git().BranchesDiffer(r.branch, r.reviewBranch)
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
	if err := r.ctx.Git().CheckoutBranch(r.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	if err := r.ctx.Git().Merge(r.branch, true); err != nil {
		return changeConflictError(err.Error())
	}
	c := r.ctx.Git().NewCommitter(r.edit)
	if err := c.Commit(message); err != nil {
		return err
	}
	return nil
}

// defaultCommitMessage creates the default commit message from the list of
// commits on the branch.
func (r *review) defaultCommitMessage() (string, error) {
	commitMessages, err := r.ctx.Git().CommitMessages(r.branch, r.reviewBranch)
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
	latestCommitMessage, err := r.ctx.Git().LatestCommitMessage()
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
		changes, err := r.ctx.Git().FilesWithUncommittedChanges()
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
	if depcopFlag {
		if err := r.checkGoDependencies(); err != nil {
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
	stashed, err := r.ctx.Git().Stash()
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer r.ctx.Run().Chdir(wd)
	topLevel, err := r.ctx.Git().TopLevel()
	if err != nil {
		return err
	}
	if err := r.ctx.Run().Chdir(topLevel); err != nil {
		return err
	}
	defer r.cleanup(stashed)
	message := ""
	data, err := ioutil.ReadFile(filename)
	if err == nil {
		message = string(data)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := r.createReviewBranch(message); err != nil {
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
	if !r.ctx.DryRun() {
		if err := r.ensureChangeID(); err != nil {
			return err
		}
	}
	if err := gerrit.Review(r.ctx, r.repo, r.draft, r.reviewers, r.ccs, r.branch); err != nil {
		return gerritError(err.Error())
	}
	return nil
}

// updateReviewMessage writes the commit message to the specified
// file. It then adds that file to the original branch, and makes sure
// it is not on the review branch.
func (r *review) updateReviewMessage(filename string) error {
	if err := r.ctx.Git().CheckoutBranch(r.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	newMessage, err := r.ctx.Git().LatestCommitMessage()
	if err != nil {
		return err
	}
	if err := r.ctx.Git().CheckoutBranch(r.branch, !gitutil.Force); err != nil {
		return err
	}
	if err := r.ctx.Run().WriteFile(filename, []byte(newMessage), 0644); err != nil {
		return fmt.Errorf("WriteFile(%v, %v) failed: %v", filename, newMessage, err)
	}
	if err := r.ctx.Git().CommitFile(filename, "Update gerrit commit message."); err != nil {
		return err
	}
	// Delete the commit message from review branch.
	if err := r.ctx.Git().CheckoutBranch(r.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	if _, err := os.Stat(filename); err == nil {
		if err := r.ctx.Git().Remove(filename); err != nil {
			return err
		}
		if err := r.ctx.Git().CommitAmend(newMessage); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// getCommitMessageFilename returns the name of the file that will get
// used for the Gerrit commit message.
func (r *review) getCommitMessageFilename() (string, error) {
	topLevel, err := r.ctx.Git().TopLevel()
	if err != nil {
		return "", err
	}
	return filepath.Join(topLevel, commitMessageFile), nil
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
	ctx := util.NewContextFromCommand(command, dryRunFlag, verboseFlag)
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
	defer ctx.Run().Chdir(wd)
	// Get the name of the current repository, if applicable.
	currentRepo, _ := ctx.Git().RepoName()
	var statuses []string
	for _, name := range names {
		if err := ctx.Run().Chdir(projects[name].Path); err != nil {
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
