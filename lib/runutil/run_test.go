package runutil

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCommandOK(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	if err := run.Command("go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`Command("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := strings.TrimSpace(out.String()), ">> go run ./testdata/ok_hello.go\nhello\n>> OK"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestCommandFail(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	if err := run.Command("go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`Command("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := strings.TrimSpace(out.String()), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	run := New(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	opts := run.Opts()
	opts.Stdout = &cmdOut
	if err := run.CommandWithOpts(opts, "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`CommandWithOpts("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := strings.TrimSpace(runOut.String()), ">> go run ./testdata/ok_hello.go\n>> OK"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestCommandWithOptsFail(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	run := New(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	opts := run.Opts()
	opts.Stdout = &cmdOut
	if err := run.CommandWithOpts(opts, "go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`CommandWithOpts("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := strings.TrimSpace(runOut.String()), ">> go run ./testdata/fail_hello.go\n>> FAILED"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestTimedCommandOK(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	if err := run.TimedCommand(10*time.Second, "go", "run", "./testdata/fast_hello.go"); err != nil {
		t.Fatalf(`TimedCommand("go run ./testdata/fast_hello.go") failed: %v`, err)
	}
	if got, want := strings.TrimSpace(out.String()), ">> go run ./testdata/fast_hello.go\nhello\n>> OK"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTimedCommandFail(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	if err := run.TimedCommand(time.Second, "go", "run", "./testdata/slow_hello.go"); err == nil {
		t.Fatalf(`TimedCommand("go run ./testdata/slow_hello.go") did not fail when it should`)
	} else if got, want := err, CommandTimedOutErr; got != want {
		t.Fatalf("unexpected error: got %v, want %v", got, want)
	}
	if got, want := strings.TrimSpace(out.String()), ">> go run ./testdata/slow_hello.go\nhello\n>> TIMED OUT"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTimedCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	run := New(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	opts := run.Opts()
	opts.Stdout = &cmdOut
	if err := run.TimedCommandWithOpts(10*time.Second, opts, "go", "run", "./testdata/fast_hello.go"); err != nil {
		t.Fatalf(`TimedCommandWithOpts("go run ./testdata/fast_hello.go") failed: %v`, err)
	}
	if got, want := strings.TrimSpace(runOut.String()), ">> go run ./testdata/fast_hello.go\n>> OK"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestTimedCommandWithOptsFail(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	run := New(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	opts := run.Opts()
	opts.Stdout = &cmdOut
	if err := run.TimedCommandWithOpts(1*time.Second, opts, "go", "run", "./testdata/slow_hello.go"); err == nil {
		t.Fatalf(`TimedCommandWithOpts("go run ./testdata/slow_hello.go") did not fail when it should`)
	} else if got, want := err, CommandTimedOutErr; got != want {
		t.Fatalf("unexpected error: got %v, want %v", got, want)
	}
	if got, want := strings.TrimSpace(runOut.String()), ">> go run ./testdata/slow_hello.go\n>> TIMED OUT"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestFunctionOK(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/ok_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := run.Function(fn, "%v %v %v", "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`Function("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := strings.TrimSpace(out.String()), ">> go run ./testdata/ok_hello.go\nhello\n>> OK"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionFail(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/fail_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := run.Function(fn, "%v %v %v", "go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`Function("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := strings.TrimSpace(out.String()), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionWithOptsOK(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, false)
	opts := run.Opts()
	opts.Verbose = true
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/ok_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := run.FunctionWithOpts(opts, fn, "%v %v %v", "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`FunctionWithOpts("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := strings.TrimSpace(out.String()), ">> go run ./testdata/ok_hello.go\nhello\n>> OK"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionWithOptsFail(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, false)
	opts := run.Opts()
	opts.Verbose = true
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/fail_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := run.FunctionWithOpts(opts, fn, "%v %v %v", "go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`FunctionWithOpts("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := strings.TrimSpace(out.String()), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestOutput(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	run.Output([]string{"hello", "world"})
	if got, want := strings.TrimSpace(out.String()), ">> hello\n>> world"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestOutputWithOpts(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, false)
	opts := run.Opts()
	opts.Verbose = true
	run.OutputWithOpts(opts, []string{"hello", "world"})
	if got, want := strings.TrimSpace(out.String()), ">> hello\n>> world"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestNested(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	fn := func() error {
		run.Output([]string{"hello", "world"})
		return nil
	}
	run.Function(fn, "%v", "greetings")
	if got, want := strings.TrimSpace(out.String()), ">> greetings\n>>>> hello\n>>>> world\n>> OK"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}
