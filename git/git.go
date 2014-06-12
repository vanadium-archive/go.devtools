package git

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"tools/cmd"
	"tools/gerrit"
)

type GitError struct {
	command string
	args    []string
	out     string
	err     string
}

func NewGitError(out, err string, args ...string) GitError {
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
	result += ge.err
	return result
}

// Add adds a file to staging.
func Add(fileName string) error {
	return run("add", fileName)
}

// AddRemote adds a new remote repository with the given name and path.
func AddRemote(name, path string) error {
	return run("remote", "add", name, path)
}

// BranchExists tests whether a branch with the given name exists in the local
// repository.
func BranchExists(branchName string) bool {
	if err := cmd.Run("git", "show-branch", branchName); err != nil {
		return false
	}
	return true
}

// BranchesDiffer tests whether two branches have any changes between them.
func BranchesDiffer(branchName1, branchName2 string) (bool, error) {
	args := []string{"diff", branchName1 + ".." + branchName2}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return false, NewGitError(out, errOut, args...)
	}
	// If output is empty, then there is no difference.
	if len(strings.Replace(out, "\n", "", -1)) == 0 {
		return false, nil
	}
	// Otherwise there is a difference.
	return true, nil
}

// CheckoutBranch checks out a branch.
func CheckoutBranch(branchName string) error {
	return run("checkout", branchName)
}

// Commit commits all files in staging with an empty message.
func Commit() error {
	return run("commit", "--allow-empty", "--allow-empty-message", "--no-edit")
}

// CommitAmend amends the previous commit with the currently staged changes,
// and the given message.
func CommitAmend(message string) error {
	return run("commit", "--amend", "-m", message)
}

// CommitAndEdit commits all files in staging and allows the user to edit the
// commit message.
func CommitAndEdit() error {
	args := []string{"commit", "--allow-empty"}
	errOut, err := cmd.RunErrorOutput("git", args...)
	if err != nil {
		return NewGitError("", errOut, args...)
	}
	return nil
}

// CommitFile commits the given file with the given commit message.
func CommitFile(fileName, message string) error {
	if err := Add(fileName); err != nil {
		return err
	}
	return CommitWithMessage(message)
}

// CommitMessages returns the concatenation of all commit messages on <branch>
// that are not also on <baseBranch>.
func CommitMessages(branch, baseBranch string) (string, error) {
	args := []string{"log", "--no-merges", baseBranch + ".." + branch}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return out, nil
}

// CommitWithMessage commits all files in staging with the given message.
func CommitWithMessage(message string) error {
	return run("commit", "--allow-empty", "--allow-empty-message", "-m", message)
}

// CommitWithMessage commits all files in staging and allows the user to edit
// the commit message. The given message will be used as the default.
func CommitWithMessageAndEdit(message string) error {
	args := []string{"commit", "--allow-empty", "-e", "-m", message}
	errOut, err := cmd.RunErrorOutput("git", args...)
	if err != nil {
		return NewGitError("", errOut, args...)
	}
	return nil
}

// CountCommits returns the number of commits on <branch> that are not on <baseBranch>.
func CountCommits(branch, baseBranch string) (int, error) {
	args := []string{"rev-list", "--count", branch, "^" + baseBranch}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return 0, NewGitError(out, errOut, args...)
	}
	count, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("Atoi(%v) failed: %v", out, err)
	}
	return count, nil
}

// CreateBranch creates a new branch with the given name.
func CreateBranch(branchName string) error {
	return run("branch", branchName)
}

// CreateAndCheckoutBranch creates a branch with the given name and checks it
// out.
func CreateAndCheckoutBranch(branchName string) error {
	return run("checkout", "-b", branchName)
}

// CreateBranchWithUpstream creates a new branch and sets the upstream repo to
// the given upstream.
func CreateBranchWithUpstream(branchName, upstream string) error {
	return run("branch", branchName, upstream)
}

// CurrentBranchName returns the name of the current branch.
func CurrentBranchName() (string, error) {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return out, nil
}

// Fetch fetches refs and tags from the remote repository.
func Fetch() error {
	return run("fetch")
}

// ForeDeleteBranch deletes the given branch, even if that branch contains
// unmerged changes.
func ForceDeleteBranch(branchName string) error {
	return run("branch", "-D", branchName)
}

// GerritRepoPath builds the URL of the Gerrit repository for the given branch.
func GerritRepoPath() (string, error) {
	repoName, err := RepoName()
	if err != nil {
		return "", err
	}
	return "https://veyron-review.googlesource.com/" + repoName, nil
}

// GerritReview pushes the branch to Gerrit.
func GerritReview(repoPathArg string, draft bool, reviewers, ccs string) error {
	repoPath := repoPathArg
	if repoPathArg == "" {
		var err error
		repoPath, err = GerritRepoPath()
		if err != nil {
			return err
		}
	}
	refspec := "HEAD:" + gerrit.Reference(draft, reviewers, ccs)
	_, errOut, err := cmd.RunOutput("git", "push", repoPath, refspec)
	if err != nil {
		return fmt.Errorf("%v", errOut)
	}
	re := regexp.MustCompile("remote:[^\n]*")
	for _, line := range re.FindAllString(errOut, -1) {
		fmt.Println(line)
	}
	return nil
}

// Init initializes a new git repo.
func Init(path string) error {
	return run("init", path)
}

// IsFileCommitted tests whether the given file has been committed to the repo.
func IsFileCommitted(file string) bool {
	// Check if file is still in staging enviroment.
	if out, _, _ := cmd.RunOutput("git", "status", "--porcelain", file); len(out) > 0 {
		return false
	}
	// Check if file is unknown to git.
	if err := cmd.Run("git", "ls-files", file, "--error-unmatch"); err != nil {
		return false
	}
	return true
}

// LatestCommitMessage returns the latest commit message on the current branch.
func LatestCommitMessage() (string, error) {
	args := []string{"log", "-n", "1", "--format=format:%B"}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return out, nil
}

// ModifiedFiles returns a slice of filenames that have changed between
// <baseBranch> and <currentBranch>.
func ModifiedFiles(baseBranch, currentBranch string) ([]string, error) {
	args := []string{"diff", "--name-only", baseBranch + ".." + currentBranch}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return nil, NewGitError(out, errOut, args...)
	}
	files := strings.Split(string(out), "\n")
	return files, nil
}

// Pull pulls the given branch from the given remote repo.
func Pull(remote, branch string) error {
	return run("pull", remote, branch)
}

// RebaseAbort aborts an in-progress rebase operation.
func RebaseAbort() error {
	return run("rebase", "--abort")
}

// Remove removes the given file.
func Remove(fileName string) error {
	return run("rm", fileName)
}

// RepoName gets the name of the current repo.
func RepoName() (string, error) {
	args := []string{"config", "--get", "remote.origin.url"}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	out, errOut, err = cmd.RunOutput("basename", out)
	if err != nil {
		return "", fmt.Errorf("'basename %v' failed:\n%v", out, errOut)
	}
	return out, nil
}

// Squash squashes all commits from <fromBranch> to the current branch.
func Squash(fromBranch string) error {
	args := []string{"merge", "--squash", fromBranch}
	if _, errOut, err := cmd.RunOutput("git", args...); err != nil {
		cmd.Run("git", "reset", "--merge")
		return fmt.Errorf("%v", errOut)
	}
	return nil
}

// Stash attempts to stash any unsaved changes. It returns true if anything was
// actually stashed, otherwise false. An error is returned if the stash command
// fails.
func Stash() (bool, error) {
	oldSize, err := StashSize()
	if err != nil {
		return false, err
	}
	if err := run("stash", "save"); err != nil {
		return false, err
	}
	newSize, err := StashSize()
	if err != nil {
		return false, err
	}
	return newSize > oldSize, nil
}

// StashSize returns the size of the stash stack.
func StashSize() (int, error) {
	args := []string{"stash", "list"}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return 0, NewGitError(out, errOut, args...)
	}
	// If output is empty, then stash is empty.
	if len(strings.Replace(out, "\n", "", -1)) == 0 {
		return 0, nil
	}
	// Otherwise, stash size is the length of the output.
	return len(strings.Split(out, "\n")), nil
}

// StashPop pops the stash into the current working tree.
func StashPop() error {
	return run("stash", "pop")
}

// TopLevel returns the top level path of the current repo.
func TopLevel() (string, error) {
	args := []string{"rev-parse", "--show-toplevel"}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return out, nil
}

func run(args ...string) error {
	out, errOut, err := cmd.RunOutput("git", args...)
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
func NewCommitter(edit bool) *Committer {
	if edit {
		return &Committer{
			commit:            CommitAndEdit,
			commitWithMessage: CommitWithMessageAndEdit,
		}
	} else {
		return &Committer{
			commit:            Commit,
			commitWithMessage: CommitWithMessage,
		}
	}
}
