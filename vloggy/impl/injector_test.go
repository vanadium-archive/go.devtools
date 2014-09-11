package impl

import (
	"go/token"
	"path"
	"strconv"
	"testing"
)

const (
	testPackagePrefix   = "tools/vloggy/impl/internal/testpackages"
	failingPrefix       = "failschecks"
	failingPackageCount = 4
)

func TestValidPackages(t *testing.T) {
	pkg := path.Join(testPackagePrefix, "passeschecks")
	_, methods := doTest(t, []string{pkg})
	if len(methods) > 0 {
		t.Fatalf("Test package %q failed to pass the log checks", pkg)
	}
}

func TestInvalidPackages(t *testing.T) {
	for i := 1; i <= failingPackageCount; i++ {
		pkg := path.Join(testPackagePrefix, failingPrefix, "test"+strconv.Itoa(i))
		_, methods := doTest(t, []string{pkg})
		if len(methods) == 0 {
			t.Fatalf("Test package %q passes log checks but it should not", pkg)
		}
	}
}

func TestInjection(t *testing.T) {
	for i := 1; i <= failingPackageCount; i++ {
		pkg := path.Join(testPackagePrefix, failingPrefix, "test"+strconv.Itoa(i))
		fset, methods := doTest(t, []string{pkg})
		if len(methods) > 0 {
			modifiedFiles := doInjection(fset, methods)
			if len(modifiedFiles) == 0 {
				t.Fatalf("Log injector did not alter any files for package %q", pkg)
			}
			for method, _ := range methods {
				if err := checkMethod(method); err != nil {
					t.Fatalf("Package %q fails log checker after injection", pkg)
				}
			}
		}
	}
}

func doTest(t *testing.T, packages []string) (*token.FileSet, map[funcDeclRef]error) {
	l := LogInjector{CheckerMode, []string{path.Join(testPackagePrefix, "iface")}, packages}

	prog, interfacePackages, implementationPackages, err := l.load()
	if err != nil {
		t.Fatal(err)
	}

	interfaces := findPublicInterfaces(interfacePackages)
	if len(interfaces) == 0 {
		t.Fatalf("Log injector did not find any interfaces in %v", interfaces)
	}

	methods := findMethodsImplementing(implementationPackages, interfaces)
	if len(methods) == 0 {
		t.Fatal("Log injector could not find any methods implementing the test interfaces")
	}

	return prog.Fset, checkMethods(methods)
}
