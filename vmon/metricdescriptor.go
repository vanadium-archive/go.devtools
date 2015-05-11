// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"sort"
	"strings"

	cloudmonitoring "google.golang.org/api/cloudmonitoring/v2beta2"

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
	Runner:   cmdline.RunnerFunc(runMetricDescriptorCreate),
	Name:     "create",
	Short:    "Create the given metric descriptor in GCM",
	Long:     "Create the given metric descriptor in GCM.",
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of metric descriptor names to create. Available: " + strings.Join(knownMetricDescriptorNames(), ", "),
}

func runMetricDescriptorCreate(env *cmdline.Env, args []string) error {
	if err := checkArgs(env, args); err != nil {
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
	fmt.Fprintf(env.Stdout, "OK\n")
	return nil
}

// cmdMetricDescriptorDelete represents the "vmon md delete" command.
var cmdMetricDescriptorDelete = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runMetricDescriptorDelete),
	Name:     "delete",
	Short:    "Delete the given metric descriptor from GCM",
	Long:     "Delete the given metric descriptor from GCM.",
	ArgsName: "<names>",
	ArgsLong: "<names> is a list of metric descriptor names to delete. Available: " + strings.Join(knownMetricDescriptorNames(), ", "),
}

func runMetricDescriptorDelete(env *cmdline.Env, args []string) error {
	if err := checkArgs(env, args); err != nil {
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
	fmt.Fprintf(env.Stdout, "OK\n")
	return nil
}

// cmdMetricDescriptorList represents the "vmon md list" command.
var cmdMetricDescriptorList = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runMetricDescriptorList),
	Name:   "list",
	Short:  "List known custom metric descriptors",
	Long:   "List known custom metric descriptors.",
}

func runMetricDescriptorList(env *cmdline.Env, _ []string) error {
	for _, n := range knownMetricDescriptorNames() {
		fmt.Fprintf(env.Stdout, "%s\n", n)
	}
	return nil
}

// cmdMetricDescriptorQuery represents the "vmon md query" command.
var cmdMetricDescriptorQuery = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runMetricDescriptorQuery),
	Name:   "query",
	Short:  "Query metric descriptors from GCM using the given filter",
	Long:   "Query metric descriptors from GCM using the given filter.",
}

func runMetricDescriptorQuery(env *cmdline.Env, _ []string) error {
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
		fmt.Fprintf(env.Stdout, "%s\n", metric.Name)
		fmt.Fprintf(env.Stdout, "- Description: %s\n", metric.Description)
		fmt.Fprintf(env.Stdout, "- Metric Type: %s\n", metric.TypeDescriptor.MetricType)
		fmt.Fprintf(env.Stdout, "- Value Type: %s\n", metric.TypeDescriptor.ValueType)
		if len(metric.Labels) > 0 {
			fmt.Fprintf(env.Stdout, "- Labels:\n")
			for _, label := range metric.Labels {
				fmt.Fprintf(env.Stdout, "  - Name: %s\n", label.Key)
				fmt.Fprintf(env.Stdout, "  - Description: %s\n", label.Description)
			}
		}
		fmt.Fprintln(env.Stdout)
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

func checkArgs(env *cmdline.Env, args []string) error {
	for _, arg := range args {
		if _, ok := monitoring.CustomMetricDescriptors[arg]; !ok {
			return env.UsageErrorf("metric descriptor %v does not exist", arg)
		}
	}
	if len(args) == 0 {
		return env.UsageErrorf("no metric descriptor provided")
	}
	return nil
}
