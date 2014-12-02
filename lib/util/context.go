package util

import (
	"io"
	"os"

	"veyron.io/lib/cmdline"
	"veyron.io/tools/lib/gitutil"
	"veyron.io/tools/lib/hgutil"
	"veyron.io/tools/lib/runutil"
)

// Context represents an execution context of a tool command
// invocation. Its purpose is to enable sharing of instances of
// various utility objects throughout the lifetime of a command
// invocation.
type Context struct {
	git *gitutil.Git
	hg  *hgutil.Hg
	run *runutil.Run
}

// NewContext returns a new context instance.
func NewContext(env map[string]string, stdin io.Reader, stdout, stderr io.Writer, dryRun, verbose bool) *Context {
	run := runutil.New(env, stdin, stdout, stderr, dryRun, verbose)
	return &Context{
		git: gitutil.New(run),
		hg:  hgutil.New(run),
		run: run,
	}
}

// NewContextFromCommand returns a new context instance based on the
// given command.
func NewContextFromCommand(command *cmdline.Command, dryRun, verbose bool) *Context {
	run := runutil.New(nil, os.Stdin, command.Stdout(), command.Stderr(), dryRun, verbose)
	return &Context{
		git: gitutil.New(run),
		hg:  hgutil.New(run),
		run: run,
	}
}

// DefaultContext returns the default context.
func DefaultContext() *Context {
	run := runutil.New(nil, os.Stdin, os.Stdout, os.Stderr, false, true)
	return &Context{
		git: gitutil.New(run),
		hg:  hgutil.New(run),
		run: run,
	}
}

// DryRun returns the dry run setting of the context.
func (ctx Context) DryRun() bool {
	return ctx.run.Opts().DryRun
}

// Env returns the environment of the context.
func (ctx Context) Env() map[string]string {
	return ctx.run.Opts().Env
}

// Git returns the git instance of the context.
func (ctx Context) Git() *gitutil.Git {
	return ctx.git
}

// Hg returns the hg instance of the context.
func (ctx Context) Hg() *hgutil.Hg {
	return ctx.hg
}

// Run returns the run instance of the context.
func (ctx Context) Run() *runutil.Run {
	return ctx.run
}

// Stdin returns the standard input of the context.
func (ctx Context) Stdin() io.Reader {
	return ctx.run.Opts().Stdin
}

// Stderr returns the standard error output of the context.
func (ctx Context) Stderr() io.Writer {
	return ctx.run.Opts().Stderr
}

// Stdout returns the standard output of the context.
func (ctx Context) Stdout() io.Writer {
	return ctx.run.Opts().Stdout
}

// Verbose returns the verbosity setting of the context.
func (ctx Context) Verbose() bool {
	return ctx.run.Opts().Verbose
}
