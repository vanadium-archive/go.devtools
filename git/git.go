package git

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"tools/cmd"
	"tools/gerrit"
)

func Add(fileName string) error {
	return cmd.Run("git", "add", fileName)
}

func AddRemote(name, path string) error {
	return cmd.Run("git", "remote", "add", name, path)
}

func BranchExists(branchName string) bool {
	if err := cmd.Run("git", "show-branch", branchName); err != nil {
		return false
	}
	return true
}

func BranchesDiffer(branchName1, branchName2 string) (bool, error) {
	out, _, err := cmd.RunOutput("git", "diff", branchName1+".."+branchName2)
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
	return cmd.Run("git", "checkout", branchName)
}

func Commit() error {
	return cmd.Run("git", "commit", "--allow-empty", "--allow-empty-message", "--no-edit")
}

func CommitAmend(message string) error {
	return cmd.Run("git", "commit", "--amend", "-m", message)
}

func CommitAndEdit() error {
	return cmd.Run("git", "commit", "--allow-empty")
}

func CommitFile(fileName, message string) error {
	if err := Add(fileName); err != nil {
		return err
	}
	return CommitWithMessage(message)
}

func CommitMessages(branch, baseBranch string) (string, error) {
	out, _, err := cmd.RunOutput("git", "log", "--no-merges", baseBranch+".."+branch)
	if err != nil {
		return "", err
	}
	return out, nil
}

func CommitWithMessage(message string) error {
	return cmd.Run("git", "commit", "--allow-empty", "--allow-empty-message", "-m", message)
}

func CommitWithMessageAndEdit(message string) error {
	return cmd.Run("git", "commit", "--allow-empty", "-e", "-m", message)
}

func CountCommits(branch, baseBranch string) (int, error) {
	out, _, err := cmd.RunOutput("git", "rev-list", "--count", branch, "^"+baseBranch)
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(out)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func CreateBranch(branchName string) error {
	return cmd.Run("git", "branch", branchName)
}

func CreateAndCheckoutBranch(branchName string) error {
	return cmd.Run("git", "checkout", "-b", branchName)
}
func CreateBranchWithUpstream(branchName, upstream string) error {
	return cmd.Run("git", "branch", branchName, upstream)
}

func CurrentBranchName() (string, error) {
	out, _, err := cmd.RunOutput("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}

func Fetch() error {
	return cmd.Run("git", "fetch")
}

func ForceDeleteBranch(branchName string) error {
	return cmd.Run("git", "branch", "-D", branchName)
}

func Init(path string) error {
	return cmd.Run("git", "init", path)
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
	out, _, err := cmd.RunOutput("git", "log", "-n", "1", "--format=format:%B")
	if err != nil {
		return "", err
	}
	return out, nil
}

func ModifiedFiles(baseBranch, currentBranch string) ([]string, error) {
	out, _, err := cmd.RunOutput("git", "diff", "--name-only", baseBranch+".."+currentBranch)
	if err != nil {
		return nil, err
	}
	files := strings.Split(string(out), "\n")
	return files, nil
}

func Pull(remote, branch string) error {
	return cmd.Run("git", "pull", remote, branch)
}

func RebaseAbort() error {
	return cmd.Run("git", "rebase", "--abort")
}

func Remove(fileName string) error {
	return cmd.Run("git", "rm", fileName)
}

func RepoName() (string, error) {
	out, _, err := cmd.RunOutput("git", "config", "--get", "remote.origin.url")
	if err != nil {
		return "", err
	}
	out, _, err = cmd.RunOutput("basename", out)
	if err != nil {
		return "", err
	}
	return out, nil
}

func Stash() error {
	return cmd.Run("git", "stash", "save")
}

func StashSize() (int, error) {
	out, _, err := cmd.RunOutput("git", "stash", "list")
	if err != nil {
		return 0, err
	}
	// If output is empty, then stash is empty.
	if len(strings.Replace(out, "\n", "", -1)) == 0 {
		return 0, nil
	}
	// Otherwise, stash size is the length of the output.
	return len(strings.Split(out, "\n")), nil
}

func StashPop() error {
	return cmd.Run("git", "stash", "pop")
}

func TopLevel() (string, error) {
	out, _, err := cmd.RunOutput("git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return out, nil
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
	_, out, err := cmd.RunOutput("git", "push", repoPath, refspec)
	if err != nil {
		return fmt.Errorf("%v", out)
	}
	re := regexp.MustCompile("remote:[^\n]*")
	for _, line := range re.FindAllString(out, -1) {
		fmt.Println(line)
	}
	return nil
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
	if err := cmd.Run("git", "merge", "--squash", from); err != nil {
		cmd.Run("git", "reset", "--merge")
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
