package git

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"tools/lib/cmd"
	"tools/lib/gerrit"
)

const (
	ROOT_ENV = "VEYRON_ROOT"
)

type GitError struct {
	command string
	args    []string
	out     []string
	err     []string
}

func NewGitError(out, err []string, args ...string) GitError {
	return GitError{
		args: args,
		out:  out,
		err:  err,
	}
}

func (ge GitError) Error() string {
	result := "'git "
	result += strings.Join(ge.args, " ")
	result += "' failed:\n"
	result += strings.Join(ge.err, "\n")
	return result
}

type Git struct {
	verbose bool
}

func New(verbose bool) *Git {
	return &Git{
		verbose: verbose,
	}
}

// Add adds a file to staging.
func (g *Git) Add(fileName string) error {
	return g.run("add", fileName)
}

// AddRemote adds a new remote repository with the given name and path.
func (g *Git) AddRemote(name, path string) error {
	return g.run("remote", "add", name, path)
}

// BranchExists tests whether a branch with the given name exists in the local
// repository.
func (g *Git) BranchExists(branchName string) bool {
	if err := cmd.Run(g.verbose, "git", "show-branch", branchName); err != nil {
		return false
	}
	return true
}

// BranchesDiffer tests whether two branches have any changes between them.
func (g *Git) BranchesDiffer(branchName1, branchName2 string) (bool, error) {
	args := []string{"--no-pager", "diff", "--name-only", branchName1 + ".." + branchName2}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return false, NewGitError(out, errOut, args...)
	}
	// If output is empty, then there is no difference.
	if len(out) == 0 {
		return false, nil
	}
	// Otherwise there is a difference.
	return true, nil
}

// CheckoutBranch checks out a branch.
func (g *Git) CheckoutBranch(branchName string) error {
	return g.run("checkout", branchName)
}

// Clone clones the given repository to the given local path.
func (g *Git) Clone(repo, path string) error {
	return g.run("clone", repo, path)
}

// Commit commits all files in staging with an empty message.
func (g *Git) Commit() error {
	return g.run("commit", "--allow-empty", "--allow-empty-message", "--no-edit")
}

// CommitAmend amends the previous commit with the currently staged changes,
// and the given message.
func (g *Git) CommitAmend(message string) error {
	return g.run("commit", "--amend", "-m", message)
}

// CommitAndEdit commits all files in staging and allows the user to edit the
// commit message.
func (g *Git) CommitAndEdit() error {
	args := []string{"commit", "--allow-empty"}
	errOut, err := cmd.RunOutputError(g.verbose, "git", args...)
	if err != nil {
		return NewGitError(nil, errOut, args...)
	}
	return nil
}

// CommitFile commits the given file with the given commit message.
func (g *Git) CommitFile(fileName, message string) error {
	if err := g.Add(fileName); err != nil {
		return err
	}
	return g.CommitWithMessage(message)
}

// CommitMessages returns the concatenation of all commit messages on <branch>
// that are not also on <baseBranch>.
func (g *Git) CommitMessages(branch, baseBranch string) (string, error) {
	args := []string{"log", "--no-merges", baseBranch + ".." + branch}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return strings.Join(out, "\n"), nil
}

// CommitWithMessage commits all files in staging with the given message.
func (g *Git) CommitWithMessage(message string) error {
	return g.run("commit", "--allow-empty", "--allow-empty-message", "-m", message)
}

// CommitWithMessage commits all files in staging and allows the user to edit
// the commit message. The given message will be used as the default.
func (g *Git) CommitWithMessageAndEdit(message string) error {
	args := []string{"commit", "--allow-empty", "-e", "-m", message}
	errOut, err := cmd.RunOutputError(g.verbose, "git", args...)
	if err != nil {
		return NewGitError(nil, errOut, args...)
	}
	return nil
}

// CountCommits returns the number of commits on <branch> that are not on <base>.
func (g *Git) CountCommits(branch, base string) (int, error) {
	args := []string{"rev-list", "--count", branch}
	if base != "" {
		args = append(args, "^"+base)
	}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return 0, NewGitError(out, errOut, args...)
	}
	if expected, got := 1, len(out); expected != got {
		return 0, NewGitError(out, errOut, args...)
	}
	count, err := strconv.Atoi(out[0])
	if err != nil {
		return 0, fmt.Errorf("Atoi(%v) failed: %v", out[0], err)
	}
	return count, nil
}

// CreateBranch creates a new branch with the given name.
func (g *Git) CreateBranch(branchName string) error {
	return g.run("branch", branchName)
}

// CreateAndCheckoutBranch creates a branch with the given name and checks it
// out.
func (g *Git) CreateAndCheckoutBranch(branchName string) error {
	return g.run("checkout", "-b", branchName)
}

// CreateBranchWithUpstream creates a new branch and sets the upstream repo to
// the given upstream.
func (g *Git) CreateBranchWithUpstream(branchName, upstream string) error {
	return g.run("branch", branchName, upstream)
}

// CurrentBranchName returns the name of the current branch.
func (g *Git) CurrentBranchName() (string, error) {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return strings.Join(out, "\n"), nil
}

// Fetch fetches refs and tags from the remote repository.
func (g *Git) Fetch() error {
	return g.run("fetch", "origin", "master")
}

// ForeDeleteBranch deletes the given branch, even if that branch contains
// unmerged changes.
func (g *Git) ForceDeleteBranch(branchName string) error {
	return g.run("branch", "-D", branchName)
}

// GerritRepoPath builds the URL of the Gerrit repository for the
// given branch.
//
// TODO(jsimsa): Move out of the git package.
func (g *Git) GerritRepoPath() (string, error) {
	repoName, err := g.RepoName()
	if err != nil {
		return "", err
	}
	return "https://veyron-review.googlesource.com/" + repoName, nil
}

// GerritReview pushes the branch to Gerrit.
//
// TODO(jsimsa): Move out of the git package.
func (g *Git) GerritReview(repoPathArg string, draft bool, reviewers, ccs, branch string) error {
	repoPath := repoPathArg
	if repoPathArg == "" {
		var err error
		repoPath, err = g.GerritRepoPath()
		if err != nil {
			return err
		}
	}
	refspec := "HEAD:" + gerrit.Reference(draft, reviewers, ccs, branch)
	_, errOut, err := cmd.RunOutput(g.verbose, "git", "push", repoPath, refspec)
	if err != nil {
		return fmt.Errorf("%v", errOut)
	}
	re := regexp.MustCompile("remote:[^\n]*")
	for _, line := range errOut {
		if re.MatchString(line) {
			fmt.Println(line)
		}
	}
	return nil
}

// Init initializes a new git repo.
func (g *Git) Init(path string) error {
	return g.run("init", path)
}

// IsFileCommitted tests whether the given file has been committed to the repo.
func (g *Git) IsFileCommitted(file string) bool {
	// Check if file is still in staging enviroment.
	if out, _, _ := cmd.RunOutput(g.verbose, "git", "status", "--porcelain", file); len(out) > 0 {
		return false
	}
	// Check if file is unknown to git.
	if err := cmd.Run(g.verbose, "git", "ls-files", file, "--error-unmatch"); err != nil {
		return false
	}
	return true
}

// LatestCommitID returns the latest commit identifier for the current branch.
func (g *Git) LatestCommitID() (string, error) {
	args := []string{"rev-parse", "HEAD"}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return strings.Join(out, "\n"), nil
}

// LatestCommitMessage returns the latest commit message on the current branch.
func (g *Git) LatestCommitMessage() (string, error) {
	args := []string{"log", "-n", "1", "--format=format:%B"}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return strings.Join(out, "\n"), nil
}

// Merge merge all commits from <branch> to the current branch. If
// <squash> is set, then all merged commits are squashed into a single
// commit.
func (g *Git) Merge(branch string, squash bool) error {
	args := []string{"merge"}
	if squash {
		args = append(args, "--squash")
	}
	args = append(args, branch)
	if out, _, err := cmd.RunOutput(g.verbose, "git", args...); err != nil {
		cmd.Run(g.verbose, "git", "reset", "--merge")
		return fmt.Errorf("%v", strings.Join(out, "\n"))
	}
	return nil
}

// ModifiedFiles returns a slice of filenames that have changed between
// <baseBranch> and <currentBranch>.
func (g *Git) ModifiedFiles(baseBranch, currentBranch string) ([]string, error) {
	args := []string{"diff", "--name-only", baseBranch + ".." + currentBranch}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return nil, NewGitError(out, errOut, args...)
	}
	return out, nil
}

// Pull pulls the given branch from the given remote repo.
func (g *Git) Pull(remote, branch string) error {
	if err := g.run("pull", remote, branch); err != nil {
		return err
	}
	major, minor, err := g.Version()
	if err != nil {
		return err
	}
	// Starting with git 1.8, "git pull remote branch" does not create
	// the remote branch "origin/master" locally. To avoid the need to
	// account for this, run "git pull", which fails but creates the
	// missing branch, for git 1.7 and older.
	if major < 2 && minor < 8 {
		g.run("pull")
	}
	return nil
}

// RebaseAbort aborts an in-progress rebase operation.
func (g *Git) RebaseAbort() error {
	return g.run("rebase", "--abort")
}

// Remove removes the given file.
func (g *Git) Remove(fileName string) error {
	return g.run("rm", fileName)
}

// RepoName gets the name of the current repo.
func (g *Git) RepoName() (string, error) {
	args := []string{"config", "--get", "remote.origin.url"}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	if expected, got := 1, len(out); expected != got {
		return "", NewGitError(out, errOut, args...)
	}
	out, errOut, err = cmd.RunOutput(g.verbose, "basename", out[0])
	if err != nil {
		return "", fmt.Errorf("'basename %v' failed:\n%v", out[0], errOut)
	}
	if expected, got := 1, len(out); expected != got {
		return "", fmt.Errorf("'basename %v' failed:\n%v", out[0], errOut)
	}
	return out[0], nil
}

// SelfUpdate updates the given tool to the latest version.
//
// TODO(jsimsa): Move out of the git package.
func (g *Git) SelfUpdate(name string) error {
	root := os.Getenv(ROOT_ENV)
	if root == "" {
		return fmt.Errorf("%v is not set", ROOT_ENV)
	}
	if _, errOut, err := cmd.RunOutput(true, "veyron", fmt.Sprintf("-v=%v", g.verbose), "project", "update", "tools"); err != nil {
		return fmt.Errorf("%s", strings.Join(errOut, "\n"))
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer os.Chdir(wd)
	repo := filepath.Join(root, "tools")
	os.Chdir(repo)
	goScript := filepath.Join(root, "veyron", "scripts", "build", "go")
	count, err := g.CountCommits("HEAD", "")
	if err != nil {
		return err
	}
	output := filepath.Join(root, "bin", name)
	ldflags := fmt.Sprintf("-X tools/%v/impl.commitId %d", name, count)
	pkg := fmt.Sprintf("tools/%v", name)
	args := []string{"build", "-ldflags", ldflags, "-o", output, pkg}
	if _, errOut, err := cmd.RunOutput(true, goScript, args...); err != nil {
		return fmt.Errorf("%v tool update failed\n%v", name, strings.Join(errOut, "\n"))
	}
	return nil
}

// SetVerbose sets the verbosity.
func (g *Git) SetVerbose(verbose bool) {
	g.verbose = verbose
}

// Stash attempts to stash any unsaved changes. It returns true if anything was
// actually stashed, otherwise false. An error is returned if the stash command
// fails.
func (g *Git) Stash() (bool, error) {
	oldSize, err := g.StashSize()
	if err != nil {
		return false, err
	}
	if err := g.run("stash", "save"); err != nil {
		return false, err
	}
	newSize, err := g.StashSize()
	if err != nil {
		return false, err
	}
	return newSize > oldSize, nil
}

// StashSize returns the size of the stash stack.
func (g *Git) StashSize() (int, error) {
	args := []string{"stash", "list"}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return 0, NewGitError(out, errOut, args...)
	}
	// If output is empty, then stash is empty.
	if len(out) == 0 {
		return 0, nil
	}
	// Otherwise, stash size is the length of the output.
	return len(out), nil
}

// StashPop pops the stash into the current working tree.
func (g *Git) StashPop() error {
	return g.run("stash", "pop")
}

// TopLevel returns the top level path of the current repo.
func (g *Git) TopLevel() (string, error) {
	args := []string{"rev-parse", "--show-toplevel"}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return strings.Join(out, "\n"), nil
}

// Version returns the major and minor git version.
func (g *Git) Version() (int, int, error) {
	args := []string{"version"}
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return 0, 0, NewGitError(out, errOut, args...)
	}
	if expected, got := 1, len(out); expected != got {
		return 0, 0, NewGitError(out, []string{fmt.Sprintf("unexpected number of lines in %v: got %v, expected %v", out, got, expected)}, args...)
	}
	words := strings.Split(out[0], " ")
	if expected, got := 3, len(words); expected > got {
		return 0, 0, NewGitError(out, []string{fmt.Sprintf("unexpected number of tokens in %v: got %v, expected at least %v", words, got, expected)}, args...)
	}
	version := strings.Split(words[2], ".")
	if expected, got := 3, len(version); expected > got {
		return 0, 0, NewGitError(out, []string{fmt.Sprintf("unexpected number of tokens in %v: got %v, expected at least %v", version, got, expected)}, args...)
	}
	major, err := strconv.Atoi(version[0])
	if err != nil {
		return 0, 0, NewGitError(out, []string{fmt.Sprintf("failed parsing %q to integer", major)}, args...)
	}
	minor, err := strconv.Atoi(version[1])
	if err != nil {
		return 0, 0, NewGitError(out, []string{fmt.Sprintf("failed parsing %q to integer", minor)}, args...)
	}
	return major, minor, nil
}

func (g *Git) run(args ...string) error {
	out, errOut, err := cmd.RunOutput(g.verbose, "git", args...)
	if err != nil {
		return NewGitError(out, errOut, args...)
	}
	return nil
}

// Committer encapsulates the process of create a commit.
type Committer struct {
	commit            func() error
	commitWithMessage func(message string) error
}

// Commit creates a commit.
func (c *Committer) Commit(message string) error {
	if len(message) == 0 {
		// No commit message supplied, let git supply one.
		return c.commit()
	}
	return c.commitWithMessage(message)
}

// NewCommitter is the Committer factory. The boolean <edit> flag
// determines whether the commit commands should prompt users to edit
// the commit message. This flag enables automated testing.
func (g *Git) NewCommitter(edit bool) *Committer {
	if edit {
		return &Committer{
			commit:            g.CommitAndEdit,
			commitWithMessage: g.CommitWithMessageAndEdit,
		}
	} else {
		return &Committer{
			commit:            g.Commit,
			commitWithMessage: g.CommitWithMessage,
		}
	}
}
