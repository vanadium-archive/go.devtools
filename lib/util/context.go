package util

import (
	"io"

	"tools/lib/gitutil"
	"tools/lib/hgutil"
	"tools/lib/runutil"
)

// Context represents an execution context of a tool command
// invocation. Its purpose is to enable sharing of instances of
// various utility objects throughout the lifetime of a command
// invocation.
type Context struct {
	git    *gitutil.Git
	hg     *hgutil.Hg
	run    *runutil.Run
	stderr io.Writer
	stdout io.Writer
}

// NewContext returns a new instance of a context.
func NewContext(verbose bool, stdout, stderr io.Writer) *Context {
	run := runutil.New(verbose, stdout)
	return &Context{
		git:    gitutil.New(run),
		hg:     hgutil.New(run),
		run:    run,
		stderr: stderr,
		stdout: stdout,
	}
}

// Git returns the git instance of the given context.
func (ctx Context) Git() *gitutil.Git {
	return ctx.git
}

// Hg returns the hg instance of the given context.
func (ctx Context) Hg() *hgutil.Hg {
	return ctx.hg
}

// Run returns the run instance of the given context.
func (ctx Context) Run() *runutil.Run {
	return ctx.run
}

// Stderr returns the standard error output of the given context.
func (ctx Context) Stderr() io.Writer {
	return ctx.stderr
}

// Stdout returns the standard output of the given context.
func (ctx Context) Stdout() io.Writer {
	return ctx.stdout
}
