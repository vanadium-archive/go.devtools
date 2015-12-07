// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TODO(jsimsa):
// - Add support for shell files without the .sh suffix.
// - Add support for Makefiles.
// - Decide what to do with the contents of the testdata directory.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri .

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/lib/cmdline"
)

func init() {
	tool.InitializeProjectFlags(&cmdCopyright.Flags)
	tool.InitializeRunFlags(&cmdCopyright.Flags)
}

const (
	defaultFileMode = os.FileMode(0644)
	hashbang        = "#!"
	jiriIgnore      = ".jiriignore"
)

var (
	copyrightRE = regexp.MustCompile(`^Copyright [[:digit:]]* The Vanadium Authors. All rights reserved.
Use of this source code is governed by a BSD-style
license that can be found in the LICENSE file.
$`)
)

type copyrightAssets struct {
	Copyright        string
	MatchFiles       map[string]string
	MatchPrefixFiles map[string]string
}

type languageSpec struct {
	CommentPrefix string
	CommentSuffix string
	Interpreters  map[string]struct{}
	FileExtension string
}

var languages map[string]languageSpec = map[string]languageSpec{
	"c_source": languageSpec{
		CommentPrefix: "// ",
		FileExtension: ".c",
	},
	"css": languageSpec{
		CommentPrefix: "/* ",
		CommentSuffix: " */",
		FileExtension: ".css",
	},
	"dart": languageSpec{
		CommentPrefix: "// ",
		FileExtension: ".dart",
	},
	"go": languageSpec{
		CommentPrefix: "// ",
		FileExtension: ".go",
	},
	"c_header": languageSpec{
		CommentPrefix: "// ",
		FileExtension: ".h",
	},
	"java": languageSpec{
		CommentPrefix: "// ",
		FileExtension: ".java",
	},
	"javascript": languageSpec{
		CommentPrefix: "// ",
		FileExtension: ".js",
	},
	"mojom": languageSpec{
		CommentPrefix: "// ",
		FileExtension: ".mojom",
	},
	"shell": languageSpec{
		CommentPrefix: "# ",
		FileExtension: ".sh",
		Interpreters: map[string]struct{}{
			"bash": struct{}{},
			"sh":   struct{}{},
		},
	},
	"vdl": languageSpec{
		CommentPrefix: "// ",
		FileExtension: ".vdl",
	},
}

// cmdCopyright represents the "jiri copyright" command.
var cmdCopyright = &cmdline.Command{
	Name:  "copyright",
	Short: "Manage vanadium copyright",
	Long: `
This command can be used to check if all source code files of Vanadium
projects contain the appropriate copyright header and also if all
projects contains the appropriate licensing files. Optionally, the
command can be used to fix the appropriate copyright headers and
licensing files.

In order to ignore checked in third-party assets which have their own copyright
and licensing headers a ".jiriignore" file can be added to a project. The
".jiriignore" file is expected to contain a single regular expression pattern per
line.
`,
	Children: []*cmdline.Command{cmdCopyrightCheck, cmdCopyrightFix},
}

// cmdCopyrightCheck represents the "jiri copyright check" command.
var cmdCopyrightCheck = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runCopyrightCheck),
	Name:     "check",
	Short:    "Check copyright headers and licensing files",
	Long:     "Check copyright headers and licensing files.",
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of projects to check.",
}

func runCopyrightCheck(jirix *jiri.X, args []string) error {
	return copyrightHelper(jirix, args, false)
}

// cmdCopyrightFix represents the "jiri copyright fix" command.
var cmdCopyrightFix = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runCopyrightFix),
	Name:     "fix",
	Short:    "Fix copyright headers and licensing files",
	Long:     "Fix copyright headers and licensing files.",
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of projects to fix.",
}

func runCopyrightFix(jirix *jiri.X, args []string) error {
	return copyrightHelper(jirix, args, true)
}

// copyrightHelper implements the logic of "jiri copyright {check,fix}".
func copyrightHelper(jirix *jiri.X, args []string, fix bool) error {
	dataDir, err := project.DataDirPath(jirix, "jiri")
	if err != nil {
		return err
	}
	assets, err := loadAssets(jirix, dataDir)
	if err != nil {
		return err
	}
	config, err := util.LoadConfig(jirix)
	if err != nil {
		return err
	}
	projects, err := project.ParseNames(jirix, args, config.CopyrightCheckProjects())
	if err != nil {
		return err
	}
	missingCopyright := false
	for _, project := range projects {
		if missing, err := checkProject(jirix, project, assets, fix); err != nil {
			return err
		} else {
			if missing {
				missingCopyright = true
			}
		}
	}
	if !fix && missingCopyright {
		return fmt.Errorf("missing copyright")
	}
	return nil
}

// createComment creates a copyright header comment out of the given
// comment symbol and copyright header data.
func createComment(prefix, suffix, header string) string {
	return prefix + strings.Replace(header, "\n", suffix+"\n"+prefix, -1) + suffix + "\n\n"
}

// checkFile checks that the given file contains the appropriate
// copyright header.
func checkFile(jirix *jiri.X, path string, assets *copyrightAssets, fix bool) (bool, error) {
	// Some projects contain third-party files in a "third_party" subdir.
	// Skip such files for the same reason that we skip the third_party project.
	if strings.Contains(path, string(filepath.Separator)+"third_party"+string(filepath.Separator)) {
		return false, nil
	}

	// Peak at the first line of the file looking for the interpreter
	// directive (e.g. #!/bin/bash).
	interpreter, err := detectInterpreter(jirix, path)
	if err != nil {
		return false, err
	}
	missingCopyright := false
	s := jirix.NewSeq()
	for _, lang := range languages {
		if _, ok := lang.Interpreters[filepath.Base(interpreter)]; ok || strings.HasSuffix(path, lang.FileExtension) {
			data, err := s.ReadFile(path)
			if err != nil {
				return false, err
			}
			if !hasCopyright(data, lang.CommentPrefix, lang.CommentSuffix) {
				if fix {
					copyright := createComment(lang.CommentPrefix, lang.CommentSuffix, assets.Copyright)
					// Add the copyright header to the beginning of the file.
					if interpreter != "" {
						// Handle the interpreter directive.
						directiveLine := hashbang + interpreter + "\n"
						data = bytes.TrimPrefix(data, []byte(directiveLine))
						copyright = directiveLine + copyright
					}
					data := append([]byte(copyright), data...)
					info, err := s.Stat(path)
					if err != nil {
						return false, err
					}
					if err := s.WriteFile(path, data, info.Mode()).Done(); err != nil {
						return false, err
					}
				} else {
					missingCopyright = true
					fmt.Fprintf(jirix.Stderr(), "%v copyright is missing\n", path)
				}
			}
		}
	}
	return missingCopyright, nil
}

// checkProject checks that the given project contains the appropriate
// licensing files and that its source code files contain the
// appropriate copyright header. If the fix option is set, the
// function fixes up the project. Otherwise, the function reports
// violations to standard error output.
func checkProject(jirix *jiri.X, project project.Project, assets *copyrightAssets, fix bool) (m bool, e error) {
	check := func(fileMap map[string]string, isValid func([]byte, []byte) bool) (bool, error) {
		s := jirix.NewSeq()
		missing := false
		for file, want := range fileMap {
			path := filepath.Join(project.Path, file)
			got, err := s.ReadFile(path)
			if err != nil {
				if runutil.IsNotExist(err) {
					if fix {
						if err := s.WriteFile(path, []byte(want), defaultFileMode).Done(); err != nil {
							return false, err
						}
					} else {
						fmt.Fprintf(jirix.Stderr(), "%v is missing\n", path)
						missing = true
					}
					continue
				} else {
					return false, err
				}
			}
			if !isValid(got, []byte(want)) {
				if fix {
					if err := s.WriteFile(path, []byte(want), defaultFileMode).Done(); err != nil {
						return false, err
					}
				} else {
					fmt.Fprintf(jirix.Stderr(), "%v is not up-to-date\n", path)
					missing = true
				}
			}
		}
		return missing, nil
	}

	missing := false
	// Check the licensing files that require an exact match.
	if missingLicense, err := check(assets.MatchFiles, bytes.Equal); err != nil {
		return false, err
	} else {
		if missingLicense {
			missing = true
		}
	}

	// Check the licensing files that require a prefix match.
	if missingLicense, err := check(assets.MatchPrefixFiles, bytes.HasPrefix); err != nil {
		return false, err
	} else {
		if missingLicense {
			missing = true
		}
	}

	// Check the source code files.
	cwd, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("Getwd() failed: %v", err)
	}
	if err := jirix.NewSeq().Chdir(project.Path).Done(); err != nil {
		return false, err
	}
	defer collect.Error(func() error { return jirix.NewSeq().Chdir(cwd).Done() }, &e)
	files, err := jirix.Git().TrackedFiles()
	if err != nil {
		return false, err
	}

	expressions, err := readV23Ignore(jirix, project)
	if err != nil {
		return false, err
	}

	for _, file := range files {
		if ignore := isIgnored(file, expressions); !ignore {
			if missingCopyright, err := checkFile(jirix, filepath.Join(project.Path, file), assets, fix); err != nil {
				return missing, err
			} else {
				if missingCopyright {
					missing = true
				}
			}
		}
	}
	return missing, nil
}

// detectInterpret returns the interpreter directive of the given
// file, if it contains one.
func detectInterpreter(jirix *jiri.X, path string) (_ string, e error) {
	file, err := jirix.NewSeq().Open(path)
	if err != nil {
		return "", err
	}
	defer collect.Error(file.Close, &e)
	// Only consider the first 256 bytes to account for binary files
	// with lines too long to fit into a memory buffer.
	data := make([]byte, 256)
	if _, err := file.Read(data); err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read %v: %v", file.Name(), err)
	}
	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	scanner.Scan()
	if err := scanner.Err(); err == nil {
		line := scanner.Text()
		if strings.HasPrefix(line, hashbang) {
			return strings.TrimPrefix(line, hashbang), nil
		}
		return "", nil
	} else {
		return "", fmt.Errorf("failed to scan %v: %v", file.Name(), err)
	}
}

// hasCopyright checks that the given byte slice contains the
// copyright header.
func hasCopyright(data []byte, prefix, suffix string) bool {
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
		lines += strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix+"\n") + "\n"
		nlines++
	}
	return copyrightRE.MatchString(lines)
}

// loadAssets returns an in-memory representation of the copyright
// assets.
func loadAssets(jirix *jiri.X, dir string) (*copyrightAssets, error) {
	result := copyrightAssets{
		MatchFiles:       map[string]string{},
		MatchPrefixFiles: map[string]string{},
	}
	s := jirix.NewSeq()
	load := func(files []string, fileMap map[string]string) error {
		for _, file := range files {
			path := filepath.Join(dir, file)
			bytes, err := s.ReadFile(path)
			if err != nil {
				return err
			}
			fileMap[file] = string(bytes)
		}
		return nil
	}
	if err := load([]string{"CONTRIBUTING.md", "LICENSE", "PATENTS", "VERSION"}, result.MatchFiles); err != nil {
		return nil, err
	}
	if err := load([]string{"AUTHORS", "CONTRIBUTORS"}, result.MatchPrefixFiles); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "COPYRIGHT")
	bytes, err := s.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result.Copyright = string(bytes)
	return &result, nil
}

// isIgnored checks a path against patterns extracted from the .jiriignore file.
func isIgnored(path string, expressions []*regexp.Regexp) bool {
	for _, expression := range expressions {
		if ok := expression.MatchString(path); ok {
			return true
		}
	}

	return false
}

func readV23Ignore(jirix *jiri.X, project project.Project) ([]*regexp.Regexp, error) {
	// Grab the .jiriignore in from project.Path. Ignore file not found errors, not
	// all projects will have one of these ignore files.
	path := filepath.Join(project.Path, jiriIgnore)
	file, err := jirix.NewSeq().Open(path)
	if err != nil {
		if !runutil.IsNotExist(err) {
			return nil, err
		}
		return nil, nil
	}
	defer file.Close()

	expressions := []*regexp.Regexp{}
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		// TODO(jsimsa): Consider implementing conventions similar to .gitignore (e.g.
		// leading '/' implies the regular expression should start with "^").
		re, err := regexp.Compile(line)
		if err != nil {
			return nil, fmt.Errorf("Compile(%v) failed: %v", line, err)
		}

		expressions = append(expressions, re)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}

	return expressions, nil
}

func main() {
	cmdline.Main(cmdCopyright)
}
