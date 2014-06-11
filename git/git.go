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

func Add(fileName string) error {
	return run("add", fileName)
}

func AddRemote(name, path string) error {
	return run("remote", "add", name, path)
}

func BranchExists(branchName string) bool {
	if err := cmd.Run("git", "show-branch", branchName); err != nil {
		return false
	}
	return true
}

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

func CheckoutBranch(branchName string) error {
	return run("checkout", branchName)
}

func Commit() error {
	return run("commit", "--allow-empty", "--allow-empty-message", "--no-edit")
}

func CommitAmend(message string) error {
	return run("commit", "--amend", "-m", message)
}

func CommitAndEdit() error {
	return run("commit", "--allow-empty")
}

func CommitFile(fileName, message string) error {
	if err := Add(fileName); err != nil {
		return err
	}
	return CommitWithMessage(message)
}

func CommitMessages(branch, baseBranch string) (string, error) {
	args := []string{"log", "--no-merges", baseBranch + ".." + branch}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return out, nil
}

func CommitWithMessage(message string) error {
	return run("commit", "--allow-empty", "--allow-empty-message", "-m", message)
}

func CommitWithMessageAndEdit(message string) error {
	return run("commit", "--allow-empty", "-e", "-m", message)
}

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

func CreateBranch(branchName string) error {
	return run("branch", branchName)
}

func CreateAndCheckoutBranch(branchName string) error {
	return run("checkout", "-b", branchName)
}
func CreateBranchWithUpstream(branchName, upstream string) error {
	return run("branch", branchName, upstream)
}

func CurrentBranchName() (string, error) {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return out, nil
}

func Fetch() error {
	return run("fetch")
}

func ForceDeleteBranch(branchName string) error {
	return run("branch", "-D", branchName)
}

func GerritRepoPath() (string, error) {
	repoName, err := RepoName()
	if err != nil {
		return "", err
	}
	return "https://veyron-review.googlesource.com/" + repoName, nil
}

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

func Init(path string) error {
	return run("init", path)
}

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

func LatestCommitMessage() (string, error) {
	args := []string{"log", "-n", "1", "--format=format:%B"}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return "", NewGitError(out, errOut, args...)
	}
	return out, nil
}

func ModifiedFiles(baseBranch, currentBranch string) ([]string, error) {
	args := []string{"diff", "--name-only", baseBranch + ".." + currentBranch}
	out, errOut, err := cmd.RunOutput("git", args...)
	if err != nil {
		return nil, NewGitError(out, errOut, args...)
	}
	files := strings.Split(string(out), "\n")
	return files, nil
}

func Pull(remote, branch string) error {
	return run("pull", remote, branch)
}

func RebaseAbort() error {
	return run("rebase", "--abort")
}

func Remove(fileName string) error {
	return run("rm", fileName)
}

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

func Squash(from string) error {
	args := []string{"merge", "--squash", from}
	if _, errOut, err := cmd.RunOutput("git", args...); err != nil {
		cmd.Run("git", "reset", "--merge")
		return fmt.Errorf("%v", errOut)
	}
	return nil
}

func Stash() error {
	return run("stash", "save")
}

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

func StashPop() error {
	return run("stash", "pop")
}

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
