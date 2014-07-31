package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var (
	depth            = 0
	ErrCommandFailed = errors.New("command failed")
)

// Log logs the result of the given command.
func Log(message string, fn func() error) error {
	LogStart(message)
	err := fn()
	LogEnd(err == nil)
	return err
}

// LogStart logs the start of the given command.
func LogStart(command string) {
	increaseDepth()
	fmt.Printf("%v %v\n", indentation(), command)
}

// LogEnd logs the outcome of a previously started command.
func LogEnd(success bool) {
	defer decreaseDepth()
	if !success {
		fmt.Printf("%v FAILED\n", indentation())
	} else {
		fmt.Printf("%v OK\n", indentation())
	}
}

// Run executes the given command with the given arguments, collecting
// no output.
func Run(verbose bool, command string, args ...string) error {
	_, _, err := run(verbose, true, command, args...)
	return err
}

// RunOutput executes the given command with the given arguments,
// collecting the standard and error output.
func RunOutput(verbose bool, command string, args ...string) ([]string, []string, error) {
	return run(verbose, true, command, args...)
}

// RunOutputError executes the given command with the given arguments,
// collecting only the error output.
func RunOutputError(verbose bool, command string, args ...string) ([]string, error) {
	_, errOut, err := run(verbose, false, command, args...)
	return errOut, err
}

func collectOutput(r io.Reader) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	lines := []string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	return lines, nil
}

func decreaseDepth() {
	depth--
}

func increaseDepth() {
	depth++
}

func indentation() string {
	result := ""
	for i := 0; i < depth; i++ {
		result += ">>"
	}
	return result
}

func run(verbose, stdout bool, command string, args ...string) ([]string, []string, error) {
	increaseDepth()
	defer decreaseDepth()
	if verbose {
		fmt.Printf("%v %v %v\n", indentation(), command, strings.Join(args, " "))
	}
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	var outPipe io.ReadCloser
	if stdout {
		var err error
		outPipe, err = cmd.StdoutPipe()
		if err != nil {
			return nil, nil, fmt.Errorf("StdoutPipe() failed: %v", err)
		}
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("StderrPipe() failed: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("Start() failed: %v", err)
	}
	out, err := collectOutput(outPipe)
	if err != nil {
		return nil, nil, err
	}
	errOut, err := collectOutput(errPipe)
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Wait(); err != nil {
		if verbose {
			fmt.Printf("%v FAILED\n", indentation())
		}
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, nil, fmt.Errorf("Wait() failed: %v", err)
		}
		return out, errOut, ErrCommandFailed
	}
	if verbose {
		for _, line := range out {
			if strings.HasPrefix(line, ">>") {
				fmt.Printf("%v%v\n", indentation(), line)
			} else {
				increaseDepth()
				fmt.Printf("%v %v\n", indentation(), line)
				decreaseDepth()
			}
		}
		fmt.Printf("%v OK\n", indentation())
	}
	return out, errOut, nil
}
