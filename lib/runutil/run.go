// Package runutil provides functions for running commands and
// functions and logging their outcome.
package runutil

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tools/lib/envutil"
)

const (
	prefix = ">>"
)

var (
	CommandTimedoutErr = fmt.Errorf("command timed out")
)

type Run struct {
	indent  int
	Stdout  io.Writer
	Verbose bool
}

func New(verbose bool, stdout io.Writer) *Run {
	return &Run{
		indent:  0,
		Stdout:  stdout,
		Verbose: verbose,
	}
}

// Command runs the given command and logs its outcome using the default verbosity.
func (r *Run) Command(stdout, stderr io.Writer, env map[string]string, path string, args ...string) error {
	return r.command(r.Verbose, stdout, stderr, env, path, 0, args...)
}

// CommandWithTimeout runs the given command with timeout and logs its outcome using the default verbosity.
func (r *Run) CommandWithTimeout(stdout, stderr io.Writer, env map[string]string, path string, timeout time.Duration, args ...string) error {
	return r.command(r.Verbose, stdout, stderr, env, path, timeout, args...)
}

// CommandWithVerbosity runs the given command and logs its outcome using the given verbosity.
func (r *Run) CommandWithVerbosity(verbose bool, stdout, stderr io.Writer, env map[string]string, path string, args ...string) error {
	return r.command(verbose, stdout, stderr, env, path, 0, args...)
}

func (r *Run) command(verbose bool, stdout, stderr io.Writer, env map[string]string, path string, timeout time.Duration, args ...string) error {
	r.increaseIndent()
	defer r.decreaseIndent()
	command := exec.Command(path, args...)
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = envutil.ToSlice(env)
	if verbose {
		args := []string{}
		for _, arg := range command.Args {
			// Quote any arguments that contain '"', ''', or ' '.
			if strings.IndexAny(arg, "\"' ") != -1 {
				args = append(args, strconv.Quote(arg))
			} else {
				args = append(args, arg)
			}
		}
		r.printf(r.Stdout, strings.Join(args, " "))
	}

	if timeout == 0 {
		// No timeout.
		var err error
		if err = command.Run(); err != nil {
			if verbose {
				r.printf(r.Stdout, "FAILED")
			}
		} else {
			if verbose {
				r.printf(r.Stdout, "OK")
			}
		}
		return err
	}
	// Has timeout.
	// Make the process of this command a new process group leader
	// to facilitate clean up of processes that time out.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		if verbose {
			r.printf(r.Stdout, "FAILED")
		}
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	select {
	case <-time.After(timeout):
		// The command has timed out.
		// Sending SIGTERM to the process group (the negative value of the process's pid)
		// will kill all the processes in that group.
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM); err != nil {
			fmt.Fprintf(r.Stdout, "Kill(%v, %v) failed: %v\n", -command.Process.Pid, syscall.SIGTERM, err)
		}
		time.Sleep(10 * time.Second)
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
			fmt.Fprintf(r.Stdout, "Kill(%v, %v) failed: %v\n", -command.Process.Pid, syscall.SIGKILL, err)
		}
		// Allow goroutine to exit.
		<-done
		if verbose {
			r.printf(r.Stdout, "TIMED OUT")
		}
		return CommandTimedoutErr
	case err := <-done:
		if err != nil {
			if verbose {
				r.printf(r.Stdout, "FAILED")
			}
		} else {
			if verbose {
				r.printf(r.Stdout, "OK")
			}
		}
		return err
	}

	return nil
}

// Function runs the given function and logs its outcome using the
// default verbosity.
func (r *Run) Function(fn func() error, format string, args ...interface{}) error {
	return r.FunctionWithVerbosity(r.Verbose, fn, format, args...)
}

// FunctionWithVerbosity runs the given function and logs its outcome
// using the given verbosity.
func (r *Run) FunctionWithVerbosity(verbose bool, fn func() error, format string, args ...interface{}) error {
	r.increaseIndent()
	defer r.decreaseIndent()
	if verbose {
		r.printf(r.Stdout, format, args...)
	}
	err := fn()
	if err != nil {
		if verbose {
			r.printf(r.Stdout, "FAILED")
		}
	} else {
		if verbose {
			r.printf(r.Stdout, "OK")
		}
	}
	return err
}

// Output logs the given list of lines using the default verbosity.
func (r *Run) Output(output []string) {
	r.OutputWithVerbosity(r.Verbose, output)
}

// OutputWithVerbosity logs the given list of lines using the given
// verbosity.
func (r *Run) OutputWithVerbosity(verbose bool, output []string) {
	if verbose {
		for _, line := range output {
			r.logLine(line)
		}
	}
}

func (r *Run) decreaseIndent() {
	r.indent--
}

func (r *Run) increaseIndent() {
	r.indent++
}

func (r *Run) logLine(line string) {
	if !strings.HasPrefix(line, prefix) {
		r.increaseIndent()
		defer r.decreaseIndent()
	}
	r.printf(r.Stdout, "%v", line)
}

func (r *Run) printf(stdout io.Writer, format string, args ...interface{}) {
	args = append([]interface{}{strings.Repeat(prefix, r.indent)}, args...)
	fmt.Fprintf(stdout, "%v "+format+"\n", args...)
}
