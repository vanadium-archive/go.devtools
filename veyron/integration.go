package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/envutil"
	"tools/lib/util"
)

// cmdIntegrationTest represents the 'integration-test' command of the
// veyron tool.
var cmdIntegrationTest = &cmdline.Command{
	Name:     "integration-test",
	Short:    "Manage integration tests",
	Long:     "Manage integration tests.",
	Children: []*cmdline.Command{cmdIntegrationTestRun, cmdIntegrationTestList},
}

// cmdIntegrationTestRun represents the 'run' sub-command of the
// 'integration-test' command of the veyron tool.
var cmdIntegrationTestRun = &cmdline.Command{
	Run:      runIntegrationTestRun,
	Name:     "run",
	Short:    "Run integration tests",
	Long:     "Run integration tests.",
	ArgsName: "<test names>",
	ArgsLong: `
<test names> is a list of short names of tests (e.g. mounttabled, playground) to
run. To see a list of tests, run the "veyron integration-test list" command.`,
}

// cmdIntegrationTestList represents the 'list' sub-command of the
// 'integration-test' command of the veyron tool.
var cmdIntegrationTestList = &cmdline.Command{
	Run:   runIntegrationTestList,
	Name:  "list",
	Short: "List available integration tests",
	Long: `
List available integration tests. Each line consists of the short test name and
the test script path relative to VEYRON_ROOT. The short test names can be used
to run individual tests in "veyron integration-test run <test names>" command.`,
}

func runIntegrationTestRun(command *cmdline.Command, args []string) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}
	ctx := util.NewContext(verboseFlag, command.Stdout(), command.Stderr())
	dir, prefix := "", "integration_test_bin_dir"
	binDir, err := ioutil.TempDir(dir, prefix)
	if err != nil {
		return fmt.Errorf("TempDir(%q, %q) failed: %v", dir, prefix, err)
	}
	if len(args) == 0 {
		// No args provided. Run all the tests.
		runArgs := []string{"go", "run"}
		// TODO(jingjin): rename shelltest-runner to integration-tester.
		runnerDir := filepath.Join(root, "tools", "go", "src", "tools", "shelltest-runner")
		fileInfoList, err := ioutil.ReadDir(runnerDir)
		if err != nil {
			return fmt.Errorf("ReadDir(%v) failed: %v", runnerDir, err)
		}
		for _, fileInfo := range fileInfoList {
			if strings.HasSuffix(fileInfo.Name(), ".go") {
				runArgs = append(runArgs, filepath.Join(runnerDir, fileInfo.Name()))
			}
		}
		runArgs = append(runArgs,
			([]string{"-bin_dir", binDir, fmt.Sprintf("-workers=%d", numTestWorkersFlag)})...)
		if err := ctx.Run().Command(command.Stdout(), command.Stderr(), nil, "veyron", runArgs...); err != nil {
			return err
		}
	} else {
		// args are the short names of tests to run.
		//
		// TODO(jingjin): now we are running these tests in sequence. Make them run parallel
		// once the build server integration is done.
		tests := findTestScripts(root)
		for _, shortTestName := range args {
			testScript, ok := tests[shortTestName]
			if !ok {
				fmt.Fprintf(command.Stderr(), "test name %q not found. Skipped.\n", shortTestName)
				continue
			}
			var stdout, stderr bytes.Buffer
			env := envutil.NewSnapshotFromOS()
			// Pass binDir to test scripts through shell_test_BIN_DIR.
			env.Set("shell_test_BIN_DIR", binDir)
			testName := trimTestScriptPath(root, testScript)
			if err := ctx.Run().Command(&stdout, &stderr, env.Map(), testScript); err != nil {
				fmt.Fprintf(command.Stdout(), "FAIL: %s\n", testName)
				fmt.Fprintf(command.Stderr(), "%s\n%s\n", stdout.String(), stderr.String())
			} else {
				fmt.Fprintf(command.Stdout(), "PASS: %s\n", testName)
			}
		}

	}
	return nil
}

func runIntegrationTestList(command *cmdline.Command, _ []string) error {
	root, err := util.VeyronRoot()
	if err != nil {
		return err
	}

	tests := findTestScripts(root)
	shortTestNames := []string{}
	for shortTestName := range tests {
		shortTestNames = append(shortTestNames, shortTestName)
	}
	sort.Strings(shortTestNames)
	for _, shortTestName := range shortTestNames {
		fmt.Fprintf(command.Stdout(), "%s (%s)\n", shortTestName, trimTestScriptPath(root, tests[shortTestName]))
	}

	return nil
}

// findTestScripts finds all the integration test scripts in veyron and returns
// a map from their short names (e.g. mounttabled, playground) to their full
// paths.
func findTestScripts(veyronRoot string) map[string]string {
	rootDirs := []string{
		filepath.Join(veyronRoot, "veyron", "go", "src"),
		filepath.Join(veyronRoot, "roadmap", "go", "src"),
	}
	tests := map[string]string{}
	for _, rootDir := range rootDirs {
		filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if strings.HasSuffix(path, string(os.PathSeparator)+"test.sh") {
				shortTestName := filepath.Base(filepath.Dir(path))
				tests[shortTestName] = path
			}
			return nil
		})
	}
	return tests
}

func trimTestScriptPath(veyronRoot, path string) string {
	testName := strings.TrimPrefix(path, veyronRoot+string(os.PathSeparator))
	return strings.TrimSuffix(testName, string(os.PathSeparator)+"test.sh")
}
