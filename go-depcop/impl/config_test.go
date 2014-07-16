package impl

import (
	"os"
	"reflect"
	"testing"
)

var invalidDependencyPolicyConfigTests = []string{
	"testdata/invalid-rules-1.depcop",
	"testdata/invalid-rules-2.depcop",
}

var dependencyPolicyConfigTests = []dependencyPolicyConfigTest{
	{"testdata/load-test.depcop", DependencyPolicy{
		Incoming: []DependencyRule{
			{IsDenyRule: false, PackageExpression: "allowed-package/x"},
		}, Outgoing: []DependencyRule{
			{IsDenyRule: true, PackageExpression: "denied-package/x"},
			{IsDenyRule: false, PackageExpression: "allowed-package/y"},
		},
	}},
	{"testdata/nacl-app.depcop", DependencyPolicy{
		Outgoing: []DependencyRule{
			{IsDenyRule: true, PackageExpression: "syscall"},
		}, Incoming: []DependencyRule{},
	}},
	{"testdata/veyron-runtimes.depcop", DependencyPolicy{
		Incoming: []DependencyRule{
			{IsDenyRule: false, PackageExpression: "veyron2/rt/..."},
			{IsDenyRule: true, PackageExpression: "..."},
		}, Outgoing: []DependencyRule{},
	}},
	{"testdata/veyron-x.depcop", DependencyPolicy{
		Incoming: []DependencyRule{
			{IsDenyRule: false, PackageExpression: "veyron/X/..."},
			{IsDenyRule: true, PackageExpression: "..."},
		}, Outgoing: []DependencyRule{},
	}},
	{"testdata/veyron2-rt.depcop", DependencyPolicy{
		Outgoing: []DependencyRule{
			{IsDenyRule: false, PackageExpression: "veyron/runtimes/..."},
		}, Incoming: []DependencyRule{},
	}},
	{"testdata/veyron2.depcop", DependencyPolicy{
		Outgoing: []DependencyRule{
			{IsDenyRule: false, PackageExpression: "veyron2/..."},
			{IsDenyRule: true, PackageExpression: "..."},
		}, Incoming: []DependencyRule{},
	}},
}

func TestLoadPackageFile(t *testing.T) {
	_, err := loadPackageConfigFile("testdata/non-existent.depcop")
	if err == nil || !os.IsNotExist(err) {
		t.Fatal("reading from a non-existent config file should return a file not exists error, got:", err)
	}

	for _, invalidFile := range invalidDependencyPolicyConfigTests {
		_, err = loadPackageConfigFile(invalidFile)
		if err == nil {
			t.Fatal("reading from the invalid config file %q is not causing an error", invalidFile)
		}
	}

	for _, tc := range dependencyPolicyConfigTests {
		pkgConfig, err := loadPackageConfigFile(tc.file)
		if err != nil {
			t.Fatal("error reading config file:", err)
		}
		if !reflect.DeepEqual(pkgConfig.Dependencies, tc.policy) {
			t.Fatalf("failed to read %q correctly. expected: %v, got: %v", tc.file, tc.policy, pkgConfig.Dependencies)
		}
	}
}

type dependencyPolicyConfigTest struct {
	file   string
	policy DependencyPolicy
}
