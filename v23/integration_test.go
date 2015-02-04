package main

import (
	"os"
	"reflect"
	"sort"
	"testing"

	"go/ast"
	"go/parser"
	"go/token"
)

func fnNames(decls []ast.Decl) []string {
	names := []string{}
	for _, d := range decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			names = append(names, fn.Name.Name)
		}
	}
	sort.Strings(names)
	return names
}

func parseFile(t *testing.T, file string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		return nil
	}
	return fnNames(f.Decls)
}

func TestIntegration(t *testing.T) {
	cases := []struct {
		dir, output        string
		internal, external []string
	}{
		{"testdata/empty", "",
			[]string{"TestHelperProcess", "TestMain"},
			[]string{"TestHelperProcess"},
		},
		{"testdata/internal_only", "", []string{"TestMain", "init"}, nil},
		{"testdata/external_only", "", nil, []string{"TestHelperProcess", "init"}},
		{"testdata/one_test", "",
			[]string{"TestHelperProcess", "init"},
			[]string{"TestV23B", "TestV23C", "init"},
		},
		{"testdata/filename", "other_test.go",
			[]string{"TestHelperProcess"},
			[]string{"TestV23B"},
		},
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmdIntegrationGenerate.Init(nil, os.Stdout, os.Stderr)
	for _, c := range cases {
		if err := os.Chdir(c.dir); err != nil {
			t.Fatal(err)
		}
		output := c.output
		if len(output) == 0 {
			output = "vanadium_integration_test.go"
		}
		if err := cmdIntegrationGenerate.Execute([]string{"--output=" + output}); err != nil {
			t.Fatal(err)
		}
		// parseFile returns nil if the file doesn't exist, which must
		// be matched by nil in the internal/external fields in the cases.
		if got, want := parseFile(t, "internal_"+output), c.internal; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got: %v, want: %#v", c.dir, got, want)
		}
		if got, want := parseFile(t, output), c.external; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got: %v, want: %#v", c.dir, got, want)
		}

		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	}
}
