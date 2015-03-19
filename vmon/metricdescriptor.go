package main

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/cloudmonitoring/v2beta2"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/lib/cmdline"
)

// cmdMetricDescriptor represents the "md" command of the vmon tool.
var cmdMetricDescriptor = &cmdline.Command{
	Name:  "md",
	Short: "The 'md' command manages metric descriptors in the given GCM instance",
	Long: `
Metric descriptor defines the metadata for a custom metric. It includes the
metric's name, description, a set of labels, and its type. Before adding custom
metric data points to GCM, we need to create its metric descriptor (once).
`,
	Children: []*cmdline.Command{
		cmdMetricDescriptorCreate,
		cmdMetricDescriptorDelete,
		cmdMetricDescriptorList,
		cmdMetricDescriptorQuery,
	},
}

// cmdMetricDescriptorCreate represents the "vmon md create" command.
var cmdMetricDescriptorCreate = &cmdline.Command{
	Run:      runMetricDescriptorCreate,
	Name:     "create",
	Short:    "Create the given metric descriptor in GCM",
	Long:     "Create the given metric descriptor in GCM.",
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of metric descriptor names to create. Available: " + strings.Join(knownMetricDescriptorNames(), ", "),
}

func runMetricDescriptorCreate(command *cmdline.Command, args []string) error {
	if err := checkArgs(command, args); err != nil {
		return err
	}

	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return err
	}
	for _, arg := range args {
		_, err := s.MetricDescriptors.Create(projectFlag, monitoring.CustomMetricDescriptors[arg]).Do()
		if err != nil {
			return fmt.Errorf("Create failed: %v", err)
		}
	}
	fmt.Fprintf(command.Stdout(), "OK\n")
	return nil
}

// cmdMetricDescriptorDelete represents the "vmon md delete" command.
var cmdMetricDescriptorDelete = &cmdline.Command{
	Run:      runMetricDescriptorDelete,
	Name:     "delete",
	Short:    "Delete the given metric descriptor from GCM",
	Long:     "Delete the given metric descriptor from GCM.",
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of metric descriptor names to delete. Available: " + strings.Join(knownMetricDescriptorNames(), ", "),
}

func runMetricDescriptorDelete(command *cmdline.Command, args []string) error {
	if err := checkArgs(command, args); err != nil {
		return err
	}

	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return err
	}
	for _, arg := range args {
		_, err := s.MetricDescriptors.Delete(projectFlag, monitoring.CustomMetricDescriptors[arg].Name).Do()
		if err != nil {
			return fmt.Errorf("Delete failed: %v", err)
		}
	}
	fmt.Fprintf(command.Stdout(), "OK\n")
	return nil
}

// cmdMetricDescriptorList represents the "vmon md list" command.
var cmdMetricDescriptorList = &cmdline.Command{
	Run:   runMetricDescriptorList,
	Name:  "list",
	Short: "List known custom metric descriptors",
	Long:  "List known custom metric descriptors.",
}

func runMetricDescriptorList(command *cmdline.Command, _ []string) error {
	for _, n := range knownMetricDescriptorNames() {
		fmt.Fprintf(command.Stdout(), "%s\n", n)
	}
	return nil
}

// cmdMetricDescriptorQuery represents the "vmon md query" command.
var cmdMetricDescriptorQuery = &cmdline.Command{
	Run:   runMetricDescriptorQuery,
	Name:  "query",
	Short: "Query metric descriptors from GCM using the given filter",
	Long:  "Query metric descriptors from GCM using the given filter.",
}

func runMetricDescriptorQuery(command *cmdline.Command, _ []string) error {
	s, err := monitoring.Authenticate(serviceAccountFlag, keyFileFlag)
	if err != nil {
		return err
	}

	// Query.
	nextPageToken := ""
	descriptors := []*cloudmonitoring.MetricDescriptor{}
	for {
		resp, err := s.MetricDescriptors.List(projectFlag, &cloudmonitoring.ListMetricDescriptorsRequest{
			Kind: "cloudmonitoring#listMetricDescriptorsRequest",
		}).Query(queryFilterFlag).PageToken(nextPageToken).Do()
		if err != nil {
			return fmt.Errorf("Query failed: %v", err)
		}
		descriptors = append(descriptors, resp.Metrics...)
		nextPageToken = resp.NextPageToken
		if nextPageToken == "" {
			break
		}
	}

	// Output results.
	for _, metric := range descriptors {
		fmt.Fprintf(command.Stdout(), "%s\n", metric.Name)
		fmt.Fprintf(command.Stdout(), "- Description: %s\n", metric.Description)
		fmt.Fprintf(command.Stdout(), "- Metric Type: %s\n", metric.TypeDescriptor.MetricType)
		fmt.Fprintf(command.Stdout(), "- Value Type: %s\n", metric.TypeDescriptor.ValueType)
		if len(metric.Labels) > 0 {
			fmt.Fprintf(command.Stdout(), "- Labels:\n")
			for _, label := range metric.Labels {
				fmt.Fprintf(command.Stdout(), "  - Name: %s\n", label.Key)
				fmt.Fprintf(command.Stdout(), "  - Description: %s\n", label.Description)
			}
		}
		fmt.Fprintln(command.Stdout())
	}

	return nil
}

func knownMetricDescriptorNames() []string {
	names := []string{}
	for n := range monitoring.CustomMetricDescriptors {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func checkArgs(command *cmdline.Command, args []string) error {
	for _, arg := range args {
		if _, ok := monitoring.CustomMetricDescriptors[arg]; !ok {
			return command.UsageErrorf("metric descriptor %v does not exist", arg)
		}
	}
	if len(args) == 0 {
		return command.UsageErrorf("no metric descriptor provided")
	}
	return nil
}
