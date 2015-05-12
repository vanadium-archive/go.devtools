// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import "v.io/x/lib/cmdline"

var (
	binDirFlag         string
	blessingsRootFlag  string
	credentialsFlag    string
	colorFlag          bool
	keyFileFlag        string
	namespaceRootFlag  string
	queryFilterFlag    string
	projectFlag        string
	serviceAccountFlag string
	verboseFlag        bool

	defaultQueryFilter = "custom.cloudmonitoring.googleapis.com"
)

func init() {
	cmdRoot.Flags.BoolVar(&colorFlag, "color", true, "Use color to format output.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.StringVar(&keyFileFlag, "key", "", "The path to the service account's key file.")
	cmdRoot.Flags.StringVar(&projectFlag, "project", "", "The GCM's corresponding GCE project ID.")
	cmdRoot.Flags.StringVar(&serviceAccountFlag, "account", "", "The service account used to communicate with GCM.")
	cmdMetricDescriptorQuery.Flags.StringVar(&queryFilterFlag, "filter", defaultQueryFilter, "The filter used for query. Default to only query custom metrics.")
	cmdCheck.Flags.StringVar(&binDirFlag, "bin-dir", "", "The path where all binaries are downloaded.")
	cmdCheck.Flags.StringVar(&blessingsRootFlag, "root", "dev.v.io", "The blessings root.")
	cmdCheck.Flags.StringVar(&namespaceRootFlag, "v23.namespace.root", "/ns.dev.v.io:8101", "The namespace root.")
	cmdCheck.Flags.StringVar(&credentialsFlag, "v23.credentials", "", "The path to v23 credentials.")
}

func main() {
	cmdline.Main(cmdRoot)
}

// cmdRoot represents the root of the vmon tool.
var cmdRoot = &cmdline.Command{
	Name:  "vmon",
	Short: "Interact with Google Cloud Monitoring",
	Long: `
Command vmon interacts with Google Cloud Monitoring.
`,
	Children: []*cmdline.Command{
		cmdMetricDescriptor,
		cmdCheck,
	},
}
