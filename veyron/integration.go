package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"tools/lib/cmdline"
	"tools/lib/util"
)

// cmdIntegrationTest represents the 'integration-test' command of the veyron tool.
var cmdIntegrationTest = &cmdline.Command{
	Name:     "integration-test",
	Short:    "Manage integration tests",
	Long:     "Manage integration tests.",
	Children: []*cmdline.Command{cmdIntegrationTestRun},
}

// cmdIntegrationTestRun represents the 'run' sub-command of the
// 'integration-test' command of the veyron tool.
// TODO(jingjin): implement the "list" sub-command and the ability to run individual tests.
var cmdIntegrationTestRun = &cmdline.Command{
	Run:   runIntegrationTestRun,
	Name:  "run",
	Short: "Run integration tests",
	Long:  "Run integration tests.",
}

func runIntegrationTestRun(command *cmdline.Command, _ []string) error {
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
	runArgs = append(runArgs, ([]string{"-bin_dir", binDir})...)
	if err := ctx.Run().Command(command.Stdout(), command.Stderr(), nil, "veyron", runArgs...); err != nil {
		return err
	}
	return nil
}
