package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

var verbose bool

// Run executes the given command with the given arguments, returning
// nil if the command succeeds, or an error otherwise.
func Run(command string, args ...string) error {
	_, _, err := RunOutput(command, args...)
	return err
}

// RunOutput executes the given command with the given arguments,
// returning the normal and error output and nil if the command
// succeeds, or an error otherwise.
func RunOutput(command string, args ...string) (string, string, error) {
	return run(command, args...)
}

// SetVerbose either enables or disables verbose output.
func SetVerbose(v bool) {
	verbose = v
}

func run(command string, args ...string) (string, string, error) {
	w := ioutil.Discard
	if verbose {
		w = os.Stdout
	}
	fmt.Fprintln(w, ">> "+command+" "+strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	var output, error bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &error
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(w, ">> FAILED")
		fmt.Fprintf(w, "%v", error.String())
		return strings.TrimSpace(output.String()), strings.TrimSpace(error.String()), fmt.Errorf("Run() failed with: %v", err)
	} else {
		fmt.Fprintln(w, ">> OK")
	}
	return strings.TrimSpace(output.String()), strings.TrimSpace(error.String()), nil
}
