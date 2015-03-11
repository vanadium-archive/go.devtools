package main

// TODO(jsimsa):
// - Add support for shell files without the .sh suffix.
// - Decide what to do with the contents of the testdata directory.

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"v.io/x/devtools/lib/util"
	"v.io/x/lib/cmdline"
)

func init() {
	cmdCopyright.Flags.StringVar(&manifestFlag, "manifest", "", "Name of the project manifest.")
}

const (
	defaultFileMode = os.FileMode(0644)
	hashbang        = "#!"
)

var (
	copyrightRE = regexp.MustCompile(`^Copyright [[:digit:]]* The Vanadium Authors. All rights reserved.
Use of this source code is governed by a BSD-style
license that can be found in the LICENSE file.
$`)
)

type copyrightAssets struct {
	Copyright string
	Files     map[string]string
}

type languageSpec struct {
	Comment      string
	Interpreters map[string]struct{}
	Suffix       string
}

var languages map[string]languageSpec = map[string]languageSpec{
	"go": languageSpec{
		Comment: "//",
		Suffix:  ".go",
	},
	"java": languageSpec{
		Comment: "//",
		Suffix:  ".java",
	},
	"javascript": languageSpec{
		Comment: "//",
		Suffix:  ".js",
	},
	"shell": languageSpec{
		Comment: "#",
		Interpreters: map[string]struct{}{
			"bash": struct{}{},
			"sh":   struct{}{},
		},
		Suffix: ".sh",
	},
}

// cmdCopyright represents the "v23 copyright" command.
var cmdCopyright = &cmdline.Command{
	Name:  "copyright",
	Short: "Manage vanadium copyright",
	Long: `
This command can be used to check if all source code files of Vanadium
projects contain the appropriate copyright header and also if all
projects contains the appropriate licensing files. Optionally, the
command can be used to fix the appropriate copyright headers and
licensing files.
`,
	Children: []*cmdline.Command{cmdCopyrightCheck, cmdCopyrightFix},
}

// cmdCopyrightCheck represents the "v23 copyright check" command.
var cmdCopyrightCheck = &cmdline.Command{
	Run:      runCopyrightCheck,
	Name:     "check",
	Short:    "Check copyright headers and licensing files",
	Long:     "Check copyright headers and licensing files.",
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of projects to check.",
}

func runCopyrightCheck(command *cmdline.Command, args []string) error {
	return copyrightHelper(command, args, false)
}

// cmdCopyrightFix represents the "v23 copyright fix" command.
var cmdCopyrightFix = &cmdline.Command{
	Run:      runCopyrightFix,
	Name:     "fix",
	Short:    "Fix copyright headers and licensing files",
	Long:     "Fix copyright headers and licensing files.",
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of projects to fix.",
}

func runCopyrightFix(command *cmdline.Command, args []string) error {
	return copyrightHelper(command, args, true)
}

// copyrightHelper implements the logic of "v23 copyright {check,fix}".
func copyrightHelper(command *cmdline.Command, args []string, fix bool) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	projects, tools, err := util.ReadManifest(ctx, manifestFlag)
	if err != nil {
		return err
	}
	names, err := parseArgs(args, projects)
	if err != nil {
		return err
	}
	dataDir := filepath.Join(projects[tools["v23"].Project].Path, tools["v23"].Data)
	assets, err := loadAssets(ctx, dataDir)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := checkProject(ctx, projects[name], assets, fix); err != nil {
			return err
		}
	}
	return nil
}

// createComment creates a copyright header comment out of the given
// comment symbol and copyright header data.
func createComment(comment, header string) string {
	return comment + " " + strings.Replace(header, "\n", "\n"+comment+" ", -1) + "\n"
}

// checkFile checks that the given file contains the appropriate
// copyright header.
func checkFile(ctx *util.Context, path string, info os.FileInfo, assets *copyrightAssets, fix bool) error {
	// Peak at the first line of the file looking for the interpreter
	// directive (e.g. #!/bin/bash).
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("Open(%v) failed: %v", path, err)
	}
	defer file.Close()
	interpreter := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, hashbang) {
			interpreter = strings.TrimPrefix(line, hashbang)
		}
	}
	for name, lang := range languages {
		if _, ok := lang.Interpreters[filepath.Base(interpreter)]; ok || strings.HasSuffix(path, lang.Suffix) {
			data, err := ctx.Run().ReadFile(path)
			if err != nil {
				return err
			}
			if !hasCopyright(data, lang.Comment) {
				if fix {
					copyright := createComment(lang.Comment, assets.Copyright)
					// Add an extra new line for Go.
					if name == "go" {
						copyright += "\n"
					}
					// Add the copyright header to the beginning of the file.
					if interpreter != "" {
						// Handle the interpreter directive.
						directiveLine := hashbang + interpreter + "\n"
						data = bytes.TrimPrefix(data, []byte(directiveLine))
						copyright = directiveLine + copyright
					}
					data := append([]byte(copyright), data...)
					if err := ctx.Run().WriteFile(path, data, info.Mode()); err != nil {
						return err
					}
				} else {
					fmt.Fprintf(ctx.Stderr(), "%v copyright is missing\n", path)
				}
			}
		}
	}
	return nil
}

// checkProject checks that the given project contains the appropriate
// licensing files and that its source code files contain the
// appropriate copyright header. If the fix option is set, the
// function fixes up the project. Otherwise, the function reports
// violations to standard error output.
func checkProject(ctx *util.Context, project util.Project, assets *copyrightAssets, fix bool) error {
	// Check the licensing files.
	for file, want := range assets.Files {
		path := filepath.Join(project.Path, file)
		got, err := ctx.Run().ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				if fix {
					if err := ctx.Run().WriteFile(path, []byte(want), defaultFileMode); err != nil {
						return err
					}
				} else {
					fmt.Fprintf(ctx.Stderr(), "%v is missing\n", path)
				}
				continue
			} else {
				return err
			}
		}
		if want != string(got) {
			if fix {
				if err := ctx.Run().WriteFile(path, []byte(want), defaultFileMode); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(ctx.Stderr(), "%v is not up-to-date\n", path)
			}
		}
	}
	// Check the copyright header.
	helper := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return checkFile(ctx, path, info, assets, fix)
	}
	if err := filepath.Walk(project.Path, helper); err != nil {
		return err
	}
	return nil
}

// hasCopyright checks that the given byte slice contains the
// copyright header.
func hasCopyright(data []byte, comment string) bool {
	buffer := bytes.NewBuffer(data)
	lines, nlines := "", 0
	for nlines < 3 {
		line, err := buffer.ReadString('\n')
		if err != nil {
			break
		}
		// Skip the interpreter directive (e.g. #!/bin/bash).
		if strings.HasPrefix(line, hashbang) {
			continue
		}
		lines += strings.TrimPrefix(line, comment+" ")
		nlines++
	}
	return copyrightRE.MatchString(lines)
}

// loadAssets returns an in-memory representation of the copyright
// assets.
func loadAssets(ctx *util.Context, dir string) (*copyrightAssets, error) {
	result := copyrightAssets{
		Files: map[string]string{},
	}
	files := []string{"AUTHORS", "CONTRIBUTORS", "LICENSE", "PATENTS", "VERSION"}
	for _, file := range files {
		path := filepath.Join(dir, file)
		bytes, err := ctx.Run().ReadFile(path)
		if err != nil {
			return nil, err
		}
		result.Files[file] = string(bytes)
	}
	path := filepath.Join(dir, "COPYRIGHT")
	bytes, err := ctx.Run().ReadFile(path)
	if err != nil {
		return nil, err
	}
	result.Copyright = string(bytes)
	return &result, nil
}

// parseArgs identifies the set of projects that the "v23 copyright
// ..." command should be applied to.
func parseArgs(args []string, projects map[string]util.Project) ([]string, error) {
	names := args
	if len(names) == 0 {
		// Use all projects (except for the third_party project) as the
		// default.
		for name, _ := range projects {
			if name != "third_party" {
				names = append(names, name)
			}
		}
	} else {
		for _, name := range names {
			if _, ok := projects[name]; !ok {
				return nil, fmt.Errorf("project %q does not exist in the project manifest", name)
			}
		}
	}
	return names, nil
}
