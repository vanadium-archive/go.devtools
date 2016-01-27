// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "text/template"

var manifestTempl = template.Must(template.New("manifest").Parse(`
<?xml version="1.0" encoding="utf-8"?>
<manifest
	xmlns:android="http://schemas.android.com/apk/res/android"
	package="{{.AndroidPackage}}"
	android:versionCode="1"
	android:versionName="1.0">

	<!-- http://developer.android.com/guide/topics/manifest/manifest-intro.html#perms  -->
	<uses-permission android:name="android.permission.INTERNET" />

	<application android:label="{{.BuildPkg.Name}}Tests" android:debuggable="true">

	<activity android:name="org.golang.app.GoNativeActivity"
		android:label="{{.BuildPkg.Name}}Tests"
		android:configChanges="orientation|keyboardHidden">
		<meta-data android:name="android.app.lib_name" android:value="{{.BuildPkg.Name}}Tests" />
		<intent-filter>
			<action android:name="android.intent.action.MAIN" />
			<category android:name="android.intent.category.LAUNCHER" />
		</intent-filter>
	</activity>
	</application>
</manifest>
`))

var mainTempl = template.Must(template.New("main").Parse(`
package main

import (
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"
	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/gl"
	{{range .FuncImports}}
	"{{.}}"{{end}}
)

var tests = []testing.InternalTest{ {{range .Tests}}
	{"{{.Name}}", {{.Package}}.{{.Name}}},{{end}}
}
var benchmarks = []testing.InternalBenchmark{ {{range .Benchmarks}}
	{"{{.Name}}", {{.Package}}.{{.Name}}},{{end}}
}
var examples = []testing.InternalExample{ {{range .Examples}}
	{"{{.Name}}", {{.Package}}.{{.Name}}},{{end}}
}

var testMain func(m *testing.M) = {{if .TestMainPackage}}{{.TestMainPackage}}.TestMain{{else}}nil{{end}}

func main() {
	fmt.Fprintf(os.Stderr, "BENDROIDPID=%d\n", os.Getpid())
	// TODO(mattr,ashankar): Also log information about the CPU (architecture, clock speed etc.)
	// and OS (version number)
	// TODO(mattr): Consider using a file to send flags to android instead of compiling
	// them into the apk.
	{{range .Flags}}
	os.Args = append(os.Args, "{{.}}"){{end}}

	go func() {
		m := testing.MainStart(regexp.MatchString, tests, benchmarks, examples)
		if testMain == nil {
			os.Exit(m.Run())
		} else {
			testMain(m)
		}
	}()
	app.Main(func(a app.App) {
		var glctx gl.Context
		ticker := time.NewTicker(time.Second / 2)
		green := false
		for {
			select {
			case <-ticker.C:
				green = !green
				a.Send(paint.Event{})
			case e := <-a.Events():
				switch e := a.Filter(e).(type) {
				case lifecycle.Event:
					glctx, _ = e.DrawContext.(gl.Context)
				case paint.Event:
					if glctx == nil {
						continue
					}
					// flashing green/blue: working
					if green {
						glctx.ClearColor(0, 1, 0, 1)
					} else {
						glctx.ClearColor(0, 0, 1, 1)
					}
					glctx.Clear(gl.COLOR_BUFFER_BIT)
					a.Publish()
				}
			}
		}
	})
}
`))
