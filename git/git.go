package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"tools/gerrit"
)

func runOutput(command string, args ...string) (string, error) {
	fmt.Println(">> " + command + " " + strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	outString := strings.TrimSpace(string(out))
	if err != nil {
		return "", errors.New(outString)
	}
	return outString, nil
}

func run(command string, args ...string) error {
	fmt.Println(">> " + command + " " + strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Run() failed with: %v", err)
	}
	return nil
}

func Add(fileName string) error {
	return run("git", "add", fileName)
}

func AddRemote(name, path string) error {
	return run("git", "remote", "add", name, path)
}

func BranchExists(branchName string) bool {
	if err := run("git", "show-branch", branchName); err != nil {
		return false
	}
	return true
}

func BranchesDiffer(branchName1, branchName2 string) (bool, error) {
	out, err := runOutput("git", "diff", branchName1+".."+branchName2)
	if err != nil {
		return false, err
	}
	// If output is empty, then there is no difference.
	if len(strings.Replace(out, "\n", "", -1)) == 0 {
		return false, nil
	}
	// Otherwise there is a difference.
	return true, nil
}

func CheckoutBranch(branchName string) error {
	return run("git", "checkout", branchName)
}

func Commit() error {
	return run("git", "commit", "--allow-empty", "--allow-empty-message", "--no-edit")
}

func CommitAmend(message string) error {
	return run("git", "commit", "--amend", "-m", message)
}

func CommitAndEdit() error {
	return run("git", "commit", "--allow-empty")
}

func CommitFile(fileName, message string) error {
	if err := Add(fileName); err != nil {
		return err
	}
	return CommitWithMessage(message)
}

func CommitMessages(branch, baseBranch string) (string, error) {
	messages, err := runOutput("git", "log", "--no-merges", baseBranch+".."+branch)
	if err != nil {
		return "", err
	}
	return messages, nil
}

func CommitWithMessage(message string) error {
	return run("git", "commit", "--allow-empty", "--allow-empty-message", "-m", message)
}

func CommitWithMessageAndEdit(message string) error {
	return run("git", "commit", "--allow-empty", "-e", "-m", message)
}

func CountCommits(branch, baseBranch string) (int, error) {
	countString, err := runOutput("git", "rev-list", "--count", branch, "^"+baseBranch)
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(countString)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func CreateBranch(branchName string) error {
	return run("git", "branch", branchName)
}

func CreateAndCheckoutBranch(branchName string) error {
	return run("git", "checkout", "-b", branchName)
}
func CreateBranchWithUpstream(branchName, upstream string) error {
	return run("git", "branch", branchName, upstream)
}

func CurrentBranchName() (string, error) {
	return runOutput("git", "rev-parse", "--abbrev-ref", "HEAD")
}

func Fetch() error {
	return run("git", "fetch")
}

func ForceDeleteBranch(branchName string) error {
	return run("git", "branch", "-D", branchName)
}

func Init(path string) error {
	return run("git", "init", path)
}

func IsFileCommitted(file string) bool {
	// Check if file is still in staging enviroment.
	if out, _ := runOutput("git", "status", "--porcelain", file); len(out) > 0 {
		return false
	}
	// Check if file is unknown to git.
	if err := run("git", "ls-files", file, "--error-unmatch"); err != nil {
		return false
	}
	return true
}

func LatestCommitMessage() (string, error) {
	return runOutput("git", "log", "-n", "1", "--format=format:%B")
}

func ModifiedFiles(baseBranch, currentBranch string) ([]string, error) {
	output, err := runOutput("git", "diff", "--name-only", baseBranch+".."+currentBranch)
	if err != nil {
		return nil, err
	}
	files := strings.Split(string(output), "\n")
	return files, nil
}

func Pull(remote, branch string) error {
	return run("git", "pull", remote, branch)
}

func RebaseAbort() error {
	return run("git", "rebase", "--abort")
}

func Remove(fileName string) error {
	return run("git", "rm", fileName)
}

func RepoName() (string, error) {
	fullName, err := runOutput("git", "config", "--get", "remote.origin.url")
	if err != nil {
		return "", err
	}
	return runOutput("basename", fullName)
}

func Stash() error {
	return run("git", "stash", "save")
}

func StashSize() (int, error) {
	out, err := runOutput("git", "stash", "list")
	if err != nil {
		return -1, err
	}
	// If output is empty, then stash is empty.
	if len(strings.Replace(out, "\n", "", -1)) == 0 {
		return 0, nil
	}
	// Otherwise, stash size is the length of the output.
	return len(strings.Split(out, "\n")), nil
}

func StashPop() error {
	return run("git", "stash", "pop")
}

func TopLevel() (string, error) {
	return runOutput("git", "rev-parse", "--show-toplevel")
}

func GerritRepoPath() (string, error) {
	repoName, err := RepoName()
	if err != nil {
		return "", err
	}
	return "https://veyron-review.googlesource.com/" + repoName, err
}

func GerritReview(repoPathArg string, draft bool, reviewers, ccs string) error {
	var repoPath string
	var err error
	if repoPathArg != "" {
		repoPath = repoPathArg
	} else {
		repoPath, err = GerritRepoPath()
		if err != nil {
			return err
		}
	}
	refspec := "HEAD:" + gerrit.Reference(draft, reviewers, ccs)
	out, err := runOutput("git", "push", repoPath, refspec)
	fmt.Println(out)
	if err != nil {
		fmt.Println(err)
	}
	return err
}

// Squasher encapsulates the process of squashing commits of the
// working git branch into a temporary review branch.
type Squasher struct {
	commit            func() error
	commitWithMessage func(message string) error
}

// Squash squashes commits from the given branch to the given branch,
// using the given message as a commit message.
func (s *Squasher) Squash(from, to, message string) error {
	CheckoutBranch(to)
	if err := run("git", "merge", "--squash", from); err != nil {
		run("git", "reset", "--merge")
		return err
	}
	if len(message) == 0 {
		// No commit message supplied, let git supply one.
		return s.commit()
	}
	return s.commitWithMessage(message)
}

// NewSquasher is the Squasher factory. The boolean <edit> flag
// determines whether the commit commands should prompt users to edit
// the commit message. This flag enables automated testing.
func NewSquasher(edit bool) *Squasher {
	if edit {
		return &Squasher{commit: CommitAndEdit,
			commitWithMessage: CommitWithMessageAndEdit}
	} else {
		return &Squasher{commit: Commit,
			commitWithMessage: CommitWithMessage}
	}
}
