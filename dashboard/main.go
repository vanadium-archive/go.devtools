// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go . -help

package main

import (
	"fmt"
	"net/http"

	"v.io/jiri/jiri"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
)

var (
	resultsBucketFlag string
	statusBucketFlag  string
	cacheFlag         string
	portFlag          int
	staticDirFlag     string
)

func init() {
	cmdDashboard.Flags.StringVar(&resultsBucketFlag, "results-bucket", resultsBucket, "Google Storage bucket to use for fetching test results.")
	cmdDashboard.Flags.StringVar(&statusBucketFlag, "status-bucket", statusBucket, "Google Storage bucket to use for fetching service status data.")
	cmdDashboard.Flags.StringVar(&cacheFlag, "cache", "", "Directory to use for caching files.")
	cmdDashboard.Flags.StringVar(&staticDirFlag, "static", "", "Directory to use for serving static files.")
	cmdDashboard.Flags.IntVar(&portFlag, "port", 8000, "Port for the server.")

	tool.InitializeRunFlags(&cmdDashboard.Flags)
}

func helper(jirix *jiri.X, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if err := validateValues(r.Form); err != nil {
		respondWithError(jirix, err, w)
		return
	}

	switch r.Form.Get("type") {
	case "presubmit":
		if err := displayPresubmitPage(jirix, w, r); err != nil {
			respondWithError(jirix, err, w)
			return
		}
		// The presubmit test results data never changes, cache it in
		// the clients for up to 30 days.
		w.Header().Set("Cache-control", "public, max-age=2592000")
	case "":
		if err := displayServiceStatusPage(jirix, w, r); err != nil {
			respondWithError(jirix, err, w)
			return
		}
	default:
		fmt.Fprintf(jirix.Stderr(), "unknown type: %v", r.Form.Get("type"))
		http.NotFound(w, r)
	}
}

func loggingHandler(jirix *jiri.X, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(jirix.Stdout(), "%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func respondWithError(jirix *jiri.X, err error, w http.ResponseWriter) {
	fmt.Fprintf(jirix.Stderr(), "%v\n", err)
	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}

func main() {
	cmdline.Main(cmdDashboard)
}

var cmdDashboard = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runDashboard),
	Name:   "dashboard",
	Short:  "Runs the Vanadium dashboard web server",
	Long:   "Command dashboard runs the Vanadium dashboard web server.",
}

func runDashboard(env *cmdline.Env, args []string) error {
	jirix, err := jiri.NewX(env)
	if err != nil {
		return err
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		helper(jirix, w, r)
	}
	health := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	staticHandler := http.FileServer(http.Dir(staticDirFlag))
	http.Handle("/static/", http.StripPrefix("/static/", staticHandler))
	http.Handle("/favicon.ico", staticHandler)
	http.HandleFunc("/health", health)
	http.HandleFunc("/", handler)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", portFlag), loggingHandler(jirix, http.DefaultServeMux)); err != nil {
		return fmt.Errorf("ListenAndServer() failed: %v", err)
	}
	return nil
}
