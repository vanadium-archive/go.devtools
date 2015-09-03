// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"text/template"

	"v.io/jiri/lib/tool"
	"v.io/x/lib/cmdline"
)

var (
	portFlag int
)

const (
	defaultRefreshMs = "5000"
)

var tmpl = template.Must(template.New("screenshot").Parse(`
<!DOCTYPE html>
<html>
	<head>
	<style>
	body {
		margin: 0px !important;
		overflow: hidden;
	}
	img {
		width: 100%;
		height: 100%;
	}
	</style>
	<script>
		function loadScreenshot() {
			var xhr = new XMLHttpRequest();
			xhr.onreadystatechange=function() {
				if (xhr.readyState == 4 && xhr.status == 200) {
					var data = JSON.parse(xhr.responseText);
					var ele = document.getElementById('screenshot');
					if (ele) {
						ele.src = 'data:image/png;base64,' + data.Data;
					}
				}
			}
			xhr.open("GET", "/data?n={{ .ScreenshotName }}", true);
			xhr.send();
		}

		function pageLoaded() {
			loadScreenshot();
			setInterval(loadScreenshot, {{ .RefreshMs }});
		}
	</script>
	</head>
	<body onload="pageLoaded()">
	<img id="screenshot"></img>
	</body>
</html>
`))

func init() {
	cmdServe.Flags.IntVar(&portFlag, "port", 8000, "Port for the server.")
}

// cmdServe represents the 'serve' command of the vkiosk tool.
var cmdServe = &cmdline.Command{
	Name:   "serve",
	Short:  "Serve screenshots from local file system or Google Storage",
	Long:   "Serve screenshots from local file system or Google Storage.",
	Runner: cmdline.RunnerFunc(runServe),
}

func runServe(env *cmdline.Env, _ []string) (e error) {
	ctx := tool.NewContextFromEnv(env)
	// Start server.
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		dataHandler(ctx, w, r)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexHandler(ctx, w, r)
	})
	if err := http.ListenAndServe(fmt.Sprintf(":%d", portFlag), nil); err != nil {
		return fmt.Errorf("ListenAndServe(%d) failed: %v", portFlag, err)
	}

	return nil
}

// indexHandler handles requests for index.html.
func indexHandler(ctx *tool.Context, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	// Parameter "n" specifies the name of the screenshot file.
	name := r.Form.Get("n")
	if name == "" {
		respondWithError(ctx, fmt.Errorf("parameter 'n' not found"), w)
		return
	}
	// Parameter "r" specifies the refresh interval.
	refreshMs := r.Form.Get("r")
	if refreshMs == "" {
		refreshMs = defaultRefreshMs
	}
	data := struct {
		ScreenshotName string
		RefreshMs      string
	}{
		ScreenshotName: name,
		RefreshMs:      refreshMs,
	}
	if err := tmpl.Execute(w, data); err != nil {
		respondWithError(ctx, fmt.Errorf("Execute() failed: %v", err), w)
		return
	}
}

// dataHandler handles requests for /data.
// It reads screenshots from local file system or Google Storage, and returns
// base64 encoded string as JSON.
func dataHandler(ctx *tool.Context, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.Form.Get("n")
	if name == "" {
		respondWithError(ctx, fmt.Errorf("parameter 'n' not found"), w)
		return
	}
	bytes, err := readScreenshot(ctx, name)
	if err != nil {
		respondWithError(ctx, fmt.Errorf("%v", err), w)
		return
	}
	encoded := base64.StdEncoding.EncodeToString(bytes)
	jsonData := struct{ Data string }{
		Data: encoded,
	}
	bytes, err = json.Marshal(&jsonData)
	if err != nil {
		respondWithError(ctx, fmt.Errorf("Marshal() failed: %v", err), w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
}

func readScreenshot(ctx *tool.Context, name string) ([]byte, error) {
	if strings.HasPrefix(exportDirFlag, "gs://") {
		args := []string{
			"-q",
			"cat",
			exportDirFlag + "/" + name,
		}
		var output bytes.Buffer
		opts := ctx.Run().Opts()
		opts.Stdout = &output
		opts.Stderr = &output
		if err := ctx.Run().CommandWithOpts(opts, "gsutil", args...); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}
	return ioutil.ReadFile(filepath.Join(exportDirFlag, name))
}

func respondWithError(ctx *tool.Context, err error, w http.ResponseWriter) {
	fmt.Fprintf(ctx.Stderr(), "%v\n", err)
	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}
