package main

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
	{"testdata/load-test.depcop", dependencyPolicy{
		Incoming: []dependencyRule{
			{IsDenyRule: false, PackageExpression: "allowed-package/x"},
		}, Outgoing: []dependencyRule{
			{IsDenyRule: true, PackageExpression: "denied-package/x"},
			{IsDenyRule: false, PackageExpression: "allowed-package/y"},
		},
	}},
	{"testdata/nacl-app.depcop", dependencyPolicy{
		Outgoing: []dependencyRule{
			{IsDenyRule: true, PackageExpression: "syscall"},
		}, Incoming: []dependencyRule{},
	}},
	{"testdata/veyron-runtimes.depcop", dependencyPolicy{
		Incoming: []dependencyRule{
			{IsDenyRule: false, PackageExpression: "veyron2/rt/..."},
			{IsDenyRule: true, PackageExpression: "..."},
		}, Outgoing: []dependencyRule{},
	}},
	{"testdata/veyron-x.depcop", dependencyPolicy{
		Incoming: []dependencyRule{
			{IsDenyRule: false, PackageExpression: "veyron/X/..."},
			{IsDenyRule: true, PackageExpression: "..."},
		}, Outgoing: []dependencyRule{},
	}},
	{"testdata/veyron2-rt.depcop", dependencyPolicy{
		Outgoing: []dependencyRule{
			{IsDenyRule: false, PackageExpression: "veyron/runtimes/..."},
		}, Incoming: []dependencyRule{},
	}},
	{"testdata/veyron2.depcop", dependencyPolicy{
		Outgoing: []dependencyRule{
			{IsDenyRule: false, PackageExpression: "veyron2/..."},
			{IsDenyRule: true, PackageExpression: "..."},
		}, Incoming: []dependencyRule{},
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
	policy dependencyPolicy
}
