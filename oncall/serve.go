// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"net/http"

	"v.io/x/devtools/internal/cache"
	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

var (
	cacheFlag     string
	portFlag      int
	staticDirFlag string
)

func init() {
	cmdServe.Flags.StringVar(&cacheFlag, "cache", "", "Directory to use for caching files.")
	cmdServe.Flags.IntVar(&portFlag, "port", 8000, "Port for the server.")
	cmdServe.Flags.StringVar(&staticDirFlag, "static", "", "Directory to use for serving static files.")
}

// cmdServe represents the 'serve' command of the oncall tool.
var cmdServe = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runServe),
	Name:   "serve",
	Short:  "Serve oncall dashboard data from Google Storage",
	Long:   "Serve oncall dashboard data from Google Storage.",
}

func runServe(env *cmdline.Env, _ []string) (e error) {
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		Verbose: &verboseFlag,
	})

	// Set up the root/cache directory.
	root := cacheFlag
	if root == "" {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		root = tmpDir
	}

	// Start server.
	handler := func(w http.ResponseWriter, r *http.Request) {
		dataHandler(ctx, root, w, r)
	}
	http.HandleFunc("/data", handler)
	staticHandler := http.FileServer(http.Dir(staticDirFlag))
	http.Handle("/", staticHandler)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", portFlag), nil); err != nil {
		return fmt.Errorf("ListenAndServe(%d) failed: %v", portFlag, err)
	}

	return nil
}

func dataHandler(ctx *tool.Context, root string, w http.ResponseWriter, r *http.Request) {
	// Get timestamp from either the "latest" file or "ts" parameter.
	r.ParseForm()
	ts := r.Form.Get("ts")
	if ts == "" {
		var err error
		ts, err = readGoogleStorageFile(ctx, "latest")
		if err != nil {
			respondWithError(ctx, err, w)
			return
		}
	}

	cachedFile, err := cache.StoreGoogleStorageFile(ctx, root, bucket, ts+".oncall")
	if err != nil {
		respondWithError(ctx, err, w)
		return
	}
	bytes, err := ctx.Run().ReadFile(cachedFile)
	if err != nil {
		respondWithError(ctx, err, w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
}

func respondWithError(ctx *tool.Context, err error, w http.ResponseWriter) {
	fmt.Fprintf(ctx.Stderr(), "%v\n", err)
	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}

func readGoogleStorageFile(ctx *tool.Context, filename string) (string, error) {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts, "gsutil", "-q", "cat", bucket+"/"+filename); err != nil {
		return "", err
	}
	return out.String(), nil
}
