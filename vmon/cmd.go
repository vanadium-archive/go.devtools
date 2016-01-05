// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"

	_ "v.io/x/ref/runtime/factories/generic"
)

var (
	binDirFlag        string
	blessingsRootFlag string
	credentialsFlag   string
	keyFileFlag       string
	namespaceRootFlag string
	queryFilterFlag   string
	projectFlag       string

	defaultQueryFilter = "custom.cloudmonitoring.googleapis.com"
)

func init() {
	cmdRoot.Flags.StringVar(&keyFileFlag, "key", "", "The path to the service account's JSON credentials file.")
	cmdRoot.Flags.StringVar(&projectFlag, "project", "", "The GCM's corresponding GCE project ID.")
	cmdMetricDescriptorQuery.Flags.StringVar(&queryFilterFlag, "filter", defaultQueryFilter, "The filter used for query. Default to only query custom metrics.")
	cmdCheck.Flags.StringVar(&binDirFlag, "bin-dir", "", "The path where all binaries are downloaded.")
	cmdCheck.Flags.StringVar(&blessingsRootFlag, "root", "dev.v.io", "The blessings root.")
	cmdCheck.Flags.StringVar(&namespaceRootFlag, "v23.namespace.root", "/ns.dev.v.io:8101", "The namespace root.")
	cmdCheck.Flags.StringVar(&credentialsFlag, "v23.credentials", "", "The path to v23 credentials.")

	tool.InitializeRunFlags(&cmdRoot.Flags)
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
