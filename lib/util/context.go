package util

import (
	"io"
	"os"

	"v.io/lib/cmdline"
	"v.io/x/devtools/lib/gerrit"
	"v.io/x/devtools/lib/gitutil"
	"v.io/x/devtools/lib/hgutil"
	"v.io/x/devtools/lib/jenkins"
	"v.io/x/devtools/lib/runutil"
)

// Context represents an execution context of a tool command
// invocation. Its purpose is to enable sharing of instances of
// various utility objects throughout the lifetime of a command
// invocation.
type Context struct {
	run *runutil.Run
}

// NewContext returns a new context instance.
func NewContext(env map[string]string, stdin io.Reader, stdout, stderr io.Writer, color, dryRun, verbose bool) *Context {
	run := runutil.New(env, stdin, stdout, stderr, color, dryRun, verbose)
	return &Context{run: run}
}

// NewContextFromCommand returns a new context instance based on the
// given command.
func NewContextFromCommand(command *cmdline.Command, color, dryRun, verbose bool) *Context {
	run := runutil.New(map[string]string{}, os.Stdin, command.Stdout(), command.Stderr(), color, dryRun, verbose)
	return &Context{run: run}
}

// DefaultContext returns the default context.
func DefaultContext() *Context {
	run := runutil.New(map[string]string{}, os.Stdin, os.Stdout, os.Stderr, false, false, true)
	return &Context{run: run}
}

// Color returns the color setting of the context.
func (ctx Context) Color() bool {
	return ctx.run.Opts().Color
}

// DryRun returns the dry run setting of the context.
func (ctx Context) DryRun() bool {
	return ctx.run.Opts().DryRun
}

// Env returns the environment of the context.
func (ctx Context) Env() map[string]string {
	return ctx.run.Opts().Env
}

// Gerrit returns the Gerrit instance of the context.
func (ctx Context) Gerrit(host, username, password string) *gerrit.Gerrit {
	return gerrit.New(host, username, password)
}

type gitOpt interface {
	gitOpt()
}
type hgOpt interface {
	hgOpt()
}
type RootDirOpt string

func (RootDirOpt) gitOpt() {}
func (RootDirOpt) hgOpt()  {}

// Git returns a new git instance.
//
// This method accepts one optional argument: the repository root to
// use for commands issued by the returned instance. If not specified,
// commands will use the current directory as the repository root.
func (ctx Context) Git(opts ...gitOpt) *gitutil.Git {
	rootDir := ""
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case RootDirOpt:
			rootDir = string(typedOpt)
		}
	}
	return gitutil.New(ctx.run, rootDir)
}

// Hg returns a new hg instance.
//
// This method accepts one optional argument: the repository root to
// use for commands issued by the returned instance. If not specified,
// commands will use the current directory as the repository root.
func (ctx Context) Hg(opts ...hgOpt) *hgutil.Hg {
	rootDir := ""
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case RootDirOpt:
			rootDir = string(typedOpt)
		}
	}
	return hgutil.New(ctx.run, rootDir)
}

// Jenkins returns a new Jenkins instance that can be used to
// communicate with a Jenkins server running at the given host.
func (ctx Context) Jenkins(host string) (*jenkins.Jenkins, error) {
	return jenkins.New(host)
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
