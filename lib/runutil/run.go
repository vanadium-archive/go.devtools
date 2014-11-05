// Package runutil provides functions for running commands and
// functions and logging their outcome.
package runutil

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
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
	indent int
	opts   Opts
}

type Opts struct {
	Env     map[string]string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Verbose bool
}

// New is the Run factory.
func New(env map[string]string, stdin io.Reader, stdout, stderr io.Writer, verbose bool) *Run {
	return &Run{
		indent: 0,
		opts: Opts{
			Env:     env,
			Stdin:   stdin,
			Stdout:  stdout,
			Stderr:  stderr,
			Verbose: verbose,
		},
	}
}

// Command runs the given command and logs its outcome using the
// default options.
func (r *Run) Command(path string, args ...string) error {
	return r.CommandWithOpts(r.opts, path, args...)
}

// CommandWithOpts runs the given command and logs its outcome using
// the given options.
func (r *Run) CommandWithOpts(opts Opts, path string, args ...string) error {
	return r.command(0, opts, path, args...)
}

// TimedCommand runs the given command with a timeout and logs its
// outcome using the default options.
func (r *Run) TimedCommand(timeout time.Duration, path string, args ...string) error {
	return r.TimedCommandWithOpts(timeout, r.opts, path, args...)
}

// TimedCommandWithOpts runs the given command with a timeout and logs
// its outcome using the given options.
func (r *Run) TimedCommandWithOpts(timeout time.Duration, opts Opts, path string, args ...string) error {
	return r.command(timeout, opts, path, args...)
}

func (r *Run) command(timeout time.Duration, opts Opts, path string, args ...string) error {
	r.increaseIndent()
	defer r.decreaseIndent()
	command := exec.Command(path, args...)
	command.Stdin = opts.Stdin
	command.Stdout = opts.Stdout
	command.Stderr = opts.Stderr
	command.Env = envutil.ToSlice(opts.Env)
	if opts.Verbose {
		args := []string{}
		for _, arg := range command.Args {
			// Quote any arguments that contain '"', ''', or ' '.
			if strings.IndexAny(arg, "\"' ") != -1 {
				args = append(args, strconv.Quote(arg))
			} else {
				args = append(args, arg)
			}
		}
		r.printf(r.opts.Stdout, strings.Join(args, " "))
	}

	if timeout == 0 {
		// No timeout.
		var err error
		if err = command.Run(); err != nil {
			if opts.Verbose {
				r.printf(r.opts.Stdout, "FAILED")
			}
		} else {
			if opts.Verbose {
				r.printf(r.opts.Stdout, "OK")
			}
		}
		return err
	}
	// Has timeout.
	// Make the process of this command a new process group leader
	// to facilitate clean up of processes that time out.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Kill this process group explicitly when receiving SIGTERM or SIGINT signals.
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigchan
		r.terminateProcessGroup(command)
	}()
	if err := command.Start(); err != nil {
		if opts.Verbose {
			r.printf(r.opts.Stdout, "FAILED")
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
		// Kill itself and its children.
		r.terminateProcessGroup(command)
		// Allow goroutine to exit.
		<-done
		if opts.Verbose {
			r.printf(r.opts.Stdout, "TIMED OUT")
		}
		return CommandTimedoutErr
	case err := <-done:
		if err != nil {
			if opts.Verbose {
				r.printf(r.opts.Stdout, "FAILED")
			}
		} else {
			if opts.Verbose {
				r.printf(r.opts.Stdout, "OK")
			}
		}
		return err
	}

	return nil
}

// terminateProcessGroup sends SIGTERM followed by SIGKILL to the process group
// (the negative value of the process's pid).
func (r *Run) terminateProcessGroup(command *exec.Cmd) {
	pid := -command.Process.Pid
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		fmt.Fprintf(r.opts.Stderr, "Kill(%v, %v) failed: %v\n", pid, syscall.SIGTERM, err)
	}
	fmt.Fprintf(r.opts.Stderr, "Waiting for command to exit: %q\n", command.Path)
	time.Sleep(10 * time.Second)
	if err := syscall.Kill(pid, 0); err == nil {
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
			fmt.Fprintf(r.opts.Stderr, "Kill(%v, %v) failed: %v\n", pid, syscall.SIGKILL, err)
		}
	}
}

// Function runs the given function and logs its outcome using the
// default verbosity.
func (r *Run) Function(fn func() error, format string, args ...interface{}) error {
	return r.FunctionWithOpts(r.opts, fn, format, args...)
}

// FunctionWithOpts runs the given function and logs its outcome using
// the given options.
func (r *Run) FunctionWithOpts(opts Opts, fn func() error, format string, args ...interface{}) error {
	r.increaseIndent()
	defer r.decreaseIndent()
	if opts.Verbose {
		r.printf(r.opts.Stdout, format, args...)
	}
	err := fn()
	if err != nil {
		if opts.Verbose {
			r.printf(r.opts.Stdout, "FAILED")
		}
	} else {
		if opts.Verbose {
			r.printf(r.opts.Stdout, "OK")
		}
	}
	return err
}

// Output logs the given list of lines using the default verbosity.
func (r *Run) Output(output []string) {
	r.OutputWithOpts(r.opts, output)
}

// OutputWithOpts logs the given list of lines using the given
// options.
func (r *Run) OutputWithOpts(opts Opts, output []string) {
	if opts.Verbose {
		for _, line := range output {
			r.logLine(line)
		}
	}
}

// Opts returns the options of the run instance.
func (r *Run) Opts() Opts {
	return r.opts
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
	r.printf(r.opts.Stdout, "%v", line)
}

func (r *Run) printf(stdout io.Writer, format string, args ...interface{}) {
	args = append([]interface{}{strings.Repeat(prefix, r.indent)}, args...)
	fmt.Fprintf(stdout, "%v "+format+"\n", args...)
}
