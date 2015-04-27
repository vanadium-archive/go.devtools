// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/storage/v1"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

var (
	colorFlag   bool
	dryRunFlag  bool
	verboseFlag bool

	osFlag         string
	archFlag       string
	datePrefixFlag string
	keyFileFlag    string
	outputDirFlag  string
)

const (
	binariesBucketName = "vanadium-binaries"
	gceUser            = "veyron"
)

// TODO(suharshs): Add tests that mock out google.Storage.

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	cmdRoot.Flags.BoolVar(&colorFlag, "color", true, "Use color to format output.")

	cmdRoot.Flags.StringVar(&archFlag, "arch", runtime.GOARCH, "Target architecture.  The default is the value of runtime.GOARCH.")
	cmdRoot.Flags.Lookup("arch").DefValue = "<runtime.GOARCH>"
	cmdRoot.Flags.StringVar(&osFlag, "os", runtime.GOOS, "Target operating system.  The default is the value of runtime.GOOS.")
	cmdRoot.Flags.Lookup("os").DefValue = "<runtime.GOOS>"

	cmdRoot.Flags.StringVar(&keyFileFlag, "key-file", "", "Google Developers service account JSON key file.")
	cmdRoot.Flags.StringVar(&datePrefixFlag, "date-prefix", "", "Date prefix to match daily build timestamps. Must be a prefix of YYYY-MM-DD.")
	cmdDownload.Flags.StringVar(&outputDirFlag, "output-dir", "", "Directory for storing downloaded binaries.")
}

// root returns a command that represents the root of the vbinary tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the "vbinary" command.
var cmdRoot = &cmdline.Command{
	Name:  "vbinary",
	Short: "Access daily builds of Vanadium binaries",
	Long: `

Command vbinary retrieves daily builds of Vanadium binaries stored in
a Google Storage bucket.
`,
	Children: []*cmdline.Command{cmdList, cmdDownload},
}

// cmdList represents the "vbinary list" command.
var cmdList = &cmdline.Command{
	Run:   runList,
	Name:  "list",
	Short: "List existing daily builds of Vanadium binaries",
	Long: `
List existing daily builds of Vanadium binaries. The displayed dates
can be limited with the --date-prefix flag.
`,
}

func runList(command *cmdline.Command, _ []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	client, err := createClient(ctx)
	if err != nil {
		return err
	}
	service, err := storage.New(client)
	if err != nil {
		return err
	}
	binaries, err := binarySnapshots(ctx, service)
	if err != nil {
		return err
	}
	for _, name := range binaries {
		fmt.Fprintf(ctx.Stdout(), "%s\n", name)
	}
	return nil
}

// cmdDownload represents the "vbinary download" command.
var cmdDownload = &cmdline.Command{
	Run:   runDownload,
	Name:  "download",
	Short: "Download an existing daily build of Vanadium binaries",
	Long: `
Download an existing daily build of Vanadium binaries. The latest
snapshot within the --date-prefix range will be downloaded. If no
--date-prefix flag is provided, the overall latest snapshot will be
downloaded.
`,
}

func runDownload(command *cmdline.Command, args []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	client, err := createClient(ctx)
	if err != nil {
		return err
	}

	binaries, timestamp, err := latestBinaries(ctx, client)
	if err != nil {
		return err
	}
	if len(outputDirFlag) == 0 {
		outputDirFlag = fmt.Sprintf("./v23_%s_%s_%s", osFlag, archFlag, timestamp)
	}
	if err := ctx.Run().MkdirAll(outputDirFlag, 0755); err != nil {
		return err
	}

	numBinaries := len(binaries)
	downloadBinaries := func() error {
		errChan := make(chan error, numBinaries)
		for _, name := range binaries {
			go downloadBinary(ctx, client, name, errChan)
		}
		gotError := false
		for i := 0; i < numBinaries; i++ {
			if err := <-errChan; err != nil {
				fmt.Fprintf(ctx.Stderr(), "failed to download binary: %v", err)
				gotError = true
			}
		}
		if gotError {
			if err := ctx.Run().RemoveAll(outputDirFlag); err != nil {
				fmt.Fprintf(ctx.Stderr(), "%v", err)
			}
			return fmt.Errorf("operation failed")
		}
		// Remove the .done file from the snapshot.
		if err := ctx.Run().RemoveAll(path.Join(outputDirFlag, ".done")); err != nil {
			return err
		}
		return nil
	}
	return ctx.Run().Function(downloadBinaries, fmt.Sprintf("Downloading binaries to %s", outputDirFlag))
}

// latestBinaries returns the binaries of the latest snapshot whose timestamp
// matches the datePrefixFlag, along with the matching timestamp.
func latestBinaries(ctx *tool.Context, client *http.Client) ([]string, string, error) {
	service, err := storage.New(client)
	if err != nil {
		return nil, "", err
	}
	timestamp, err := latestTimestamp(ctx, client, service)
	if err != nil {
		return nil, "", err
	}
	binaryPrefix := fmt.Sprintf("%s_%s/%s", osFlag, archFlag, timestamp)
	res, err := service.Objects.List(binariesBucketName).Fields("nextPageToken", "items/name").Prefix(binaryPrefix).Do()
	if err != nil {
		return nil, "", err
	}
	objs := res.Items
	for res.NextPageToken != "" {
		res, err = service.Objects.List(binariesBucketName).PageToken(res.NextPageToken).Do()
		if err != nil {
			return nil, "", err
		}
		objs = append(objs, res.Items...)
	}
	if len(objs) == 0 {
		return nil, "", fmt.Errorf("no binaries found (OS: %s, Arch: %s, Date: %s)", osFlag, archFlag, timestamp)
	}
	ret := make([]string, len(objs))
	for i, obj := range objs {
		ret[i] = obj.Name
	}
	return ret, timestamp, nil
}

// latestTimestamp returns the time of the latest snapshot within the
// date-prefix range.
func latestTimestamp(ctx *tool.Context, client *http.Client, service *storage.Service) (string, error) {
	// If no datePrefixFlag is provided, we just want to get the latest snapshot.
	if datePrefixFlag == "" {
		latestFile := fmt.Sprintf("%s_%s/%s", osFlag, archFlag, "latest")
		b, err := downloadFileBytes(client, latestFile)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	// Otherwise, we get the snapshots that match datePrefixFlag and choose the latest.
	snapshots, err := binarySnapshots(ctx, service)
	if err != nil {
		return "", err
	}
	const layout = "2006-01-02T15:04:05-07:00"
	var latest string
	var latestTime time.Time
	for _, name := range snapshots {
		spl := strings.Split(name, "/")
		timestamp := spl[1]
		t, err := time.Parse(layout, timestamp)
		if err != nil {
			return "", err
		}
		if t.After(latestTime) {
			latest = timestamp
			latestTime = t
		}
	}
	return latest, nil
}

func binarySnapshots(ctx *tool.Context, service *storage.Service) ([]string, error) {
	filterSnapshots := func(call *storage.ObjectsListCall) (*storage.Objects, error) {
		binaryPrefix := fmt.Sprintf("%s_%s/%s", osFlag, archFlag, datePrefixFlag)
		// We delimit results by the ".done" file to ensure that only successfully completed snapshots are considered.
		return call.Fields("nextPageToken", "prefixes").Prefix(binaryPrefix).Delimiter("/.done").Do()
	}
	res, err := filterSnapshots(service.Objects.List(binariesBucketName))
	if err != nil {
		return nil, err
	}
	snapshots := res.Prefixes
	for res.NextPageToken != "" {
		res, err = filterSnapshots(service.Objects.List(binariesBucketName).PageToken(res.NextPageToken))
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, res.Prefixes...)
	}
	if len(snapshots) == 0 {
		fmt.Fprintf(ctx.Stderr(), "no snapshots found (OS: %s, Arch: %s, Date: %s)\n", osFlag, archFlag, datePrefixFlag)
		return nil, cmdline.ErrExitCode(util.NoSnapshotExitCode)
	}
	ret := make([]string, len(snapshots))
	for i, snapshot := range snapshots {
		ret[i] = strings.TrimSuffix(snapshot, "/.done")
	}
	return ret, nil
}

func createClient(ctx *tool.Context) (*http.Client, error) {
	if os.Getenv("USER") == gceUser {
		// We're on GCE, use the metadata server to get a token.
		return oauth2.NewClient(oauth2.NoContext, google.ComputeTokenSource("")), nil
	}

	if len(keyFileFlag) > 0 {
		data, err := ctx.Run().ReadFile(keyFileFlag)
		if err != nil {
			return nil, err
		}
		conf, err := google.JWTConfigFromJSON(data, storage.CloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("failed to create JWT config file: %v", err)
		}
		return conf.Client(oauth2.NoContext), nil
	}

	return &http.Client{}, nil
}

func downloadBinary(ctx *tool.Context, client *http.Client, binaryPath string, errChan chan<- error) {
	helper := func() error {
		b, err := downloadFileBytes(client, binaryPath)
		if err != nil {
			return fmt.Errorf("failed to download file %v: %v", binaryPath, err)
		}
		fileName := filepath.Join(outputDirFlag, path.Base(binaryPath))
		if err := ctx.Run().WriteFile(fileName, b, 0755); err != nil {
			return err
		}
		return nil
	}
	errChan <- helper()
}

func downloadFileBytes(client *http.Client, filePath string) (b []byte, e error) {
	// This roundabout request is required because of the issue detailed here:
	// https://plus.sandbox.google.com/+IanRose/posts/Tzw3QZqEQZk
	// and here:
	// https://groups.google.com/forum/#!msg/Golang-nuts/juguXl-ss2Q/oOVFvHYqoSgJ.
	urls := "https://www.googleapis.com/download/storage/v1/b/{bucket}/o/{object}?alt=media"
	req, err := http.NewRequest("GET", urls, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to %s: %v\n", urls, err)
	}
	req.URL.Path = strings.Replace(req.URL.Path, "{bucket}", url.QueryEscape(binariesBucketName), 1)
	req.URL.Path = strings.Replace(req.URL.Path, "{object}", url.QueryEscape(filePath), 1)
	googleapi.SetOpaque(req.URL)
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download %v: %v\n", req.URL.RequestURI(), err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got StatusCode %v for download %v", req.URL.RequestURI(), res.StatusCode)
	}
	defer collect.Error(func() error { return res.Body.Close() }, &e)

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(res.Body); err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}
	return buf.Bytes(), nil
}
