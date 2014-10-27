// Package hgutil provides Go wrappers for a variety of mercurial
// commands.
package hgutil

import (
	"bytes"
	"fmt"
	"strings"

	"tools/lib/runutil"
)

type HgError struct {
	args        []string
	output      string
	errorOutput string
}

func Error(output, errorOutput string, args ...string) HgError {
	return HgError{
		args:        args,
		output:      output,
		errorOutput: errorOutput,
	}
}

func (he HgError) Error() string {
	result := "'hg "
	result += strings.Join(he.args, " ")
	result += "' failed:\n"
	result += he.errorOutput
	return result
}

type Hg struct {
	runner *runutil.Run
}

func New(runner *runutil.Run) *Hg {
	return &Hg{
		runner: runner,
	}
}

// CheckoutBranch switches the current repository to the given branch.
func (h *Hg) CheckoutBranch(branch string) error {
	return h.run("update", branch)
}

// CheckoutRevision switches the revision for the current repository.
func (h *Hg) CheckoutRevision(revision string) error {
	return h.run("update", "-r", revision)
}

// Clone clones the given repository to the given local path.
func (h *Hg) Clone(repo, path string) error {
	return h.run("clone", repo, path)
}

// CurrentBranchName returns the name of the current branch.
func (h *Hg) CurrentBranchName() (string, error) {
	out, err := h.runOutput("branch")
	if err != nil {
		return "", err
	}
	if expected, got := 1, len(out); expected != got {
		return "", fmt.Errorf("unexpected length of %v: expected %v, got %v", out, expected, got)
	}
	return strings.Join(out, "\n"), nil
}

// GetBranches returns a slice of the local branches of the current
// repository, followed by the name of the current branch.
func (h *Hg) GetBranches() ([]string, string, error) {
	current, err := h.CurrentBranchName()
	if err != nil {
		return nil, "", err
	}
	out, err := h.runOutput("branches")
	if err != nil {
		return nil, "", err
	}
	branches := []string{}
	for _, branch := range out {
		branches = append(branches, strings.TrimSpace(branch))
	}
	return branches, current, nil
}

// Pull updates the current branch from the remote repository.
func (h *Hg) Pull() error {
	return h.run("pull", "-u")
}

// RepoName gets the name of the current repository.
func (h *Hg) RepoName() (string, error) {
	out, err := h.runOutput("paths", "default")
	if err != nil {
		return "", err
	}
	if expected, got := 1, len(out); expected != got {
		return "", fmt.Errorf("unexpected length of %v: expected %v, got %v", out, expected, got)
	}
	return out[0], nil
}

func (h *Hg) run(args ...string) error {
	var stdout, stderr bytes.Buffer
	if err := h.runner.Command(&stdout, &stderr, nil, "hg", args...); err != nil {
		return Error(stdout.String(), stderr.String(), args...)
	}
	return nil
}

func (h *Hg) runOutput(args ...string) ([]string, error) {
	var stdout, stderr bytes.Buffer
	if err := h.runner.Command(&stdout, &stderr, nil, "hg", args...); err != nil {
		return nil, Error(stdout.String(), stderr.String(), args...)
	}
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil
	} else {
		return strings.Split(output, "\n"), nil
	}
}
