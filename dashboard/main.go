// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Daemon dashboard implements the Vanadium dashboard web server.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"v.io/x/devtools/internal/tool"
)

var (
	resultsBucketFlag string
	statusBucketFlag  string
	cacheFlag         string
	dryRunFlag        bool
	colorFlag         bool
	portFlag          int
	staticDirFlag     string
	verboseFlag       bool
)

func init() {
	flag.StringVar(&resultsBucketFlag, "results-bucket", resultsBucket, "Google Storage bucket to use for fetching test results.")
	flag.StringVar(&statusBucketFlag, "status-bucket", statusBucket, "Google Storage bucket to use for fetching service status data.")
	flag.StringVar(&cacheFlag, "cache", "", "Directory to use for caching files.")
	flag.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	flag.BoolVar(&colorFlag, "color", true, "Use color to format output.")
	flag.StringVar(&staticDirFlag, "static", "", "Directory to use for serving static files.")
	flag.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	flag.IntVar(&portFlag, "port", 8000, "Port for the server.")
	flag.Parse()
}

func helper(ctx *tool.Context, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if err := validateValues(r.Form); err != nil {
		respondWithError(ctx, err, w)
		return
	}

	switch r.Form.Get("type") {
	case "presubmit":
		if err := displayPresubmitPage(ctx, w, r); err != nil {
			respondWithError(ctx, err, w)
			return
		}
		// The presubmit test results data never changes, cache it in
		// the clients for up to 30 days.
		w.Header().Set("Cache-control", "public, max-age=2592000")
	case "":
		if err := displayServiceStatusPage(ctx, w, r); err != nil {
			respondWithError(ctx, err, w)
			return
		}
	default:
		fmt.Fprintf(ctx.Stderr(), "unknown type: %v", r.Form.Get("type"))
		http.NotFound(w, r)
	}
}

func loggingHandler(ctx *tool.Context, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(ctx.Stdout(), "%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func respondWithError(ctx *tool.Context, err error, w http.ResponseWriter) {
	fmt.Fprintf(ctx.Stderr(), "%v\n", err)
	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}

func main() {
	ctx := tool.NewContext(tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	handler := func(w http.ResponseWriter, r *http.Request) {
		helper(ctx, w, r)
	}
	health := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	staticHandler := http.FileServer(http.Dir(staticDirFlag))
	http.Handle("/static/", http.StripPrefix("/static/", staticHandler))
	http.Handle("/favicon.ico", staticHandler)
	http.HandleFunc("/health", health)
	http.HandleFunc("/", handler)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", portFlag), loggingHandler(ctx, http.DefaultServeMux)); err != nil {
		fmt.Fprintf(os.Stderr, "ListenAndServer() failed: %v", err)
		os.Exit(1)
	}
}
