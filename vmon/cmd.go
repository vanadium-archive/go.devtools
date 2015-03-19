package main

import "v.io/x/lib/cmdline"

var (
	binDirFlag         string
	blessingsRootFlag  string
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
	cmdCheck.Flags.StringVar(&binDirFlag, "bin_dir", "", "The path where all binaries are downloaded.")
	cmdCheck.Flags.StringVar(&blessingsRootFlag, "root", "dev.v.io", "The blessings root.")
	cmdCheck.Flags.StringVar(&namespaceRootFlag, "ns", "/ns.dev.v.io:8101", "The namespace root.")

	services = []prodService{
		prodService{
			name:       "mounttable",
			objectName: namespaceRootFlag,
		},
		prodService{
			name:       "application repository",
			objectName: namespaceRootFlag + "/applications",
		},
		prodService{
			name:       "binary repository",
			objectName: namespaceRootFlag + "/binaries",
		},
		prodService{
			name:       "macaroon service",
			objectName: namespaceRootFlag + "/identity/" + blessingsRootFlag + "/root/macaroon",
		},
		prodService{
			name:       "google identity service",
			objectName: namespaceRootFlag + "/identity/" + blessingsRootFlag + "/root/google",
		},
		prodService{
			name:       "binary discharger",
			objectName: namespaceRootFlag + "/identity/" + blessingsRootFlag + "/root/discharger",
		},
	}
}

// root returns a command that represents the root of the vmon tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the vmon tool.
var cmdRoot = &cmdline.Command{
	Name:  "vmon",
	Short: "Tool for interacting with Google Cloud Monitoring (GCM)",
	Long:  "The vmon tool performs various operatios using GCM APIs.",
	Children: []*cmdline.Command{
		cmdMetricDescriptor,
		cmdCheck,
	},
}
