// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
)

var javaSrcFile = filepath.Join("src", "main", "java", "io", "v", "x", "devtools", "bendroid", "BendroidActivity.java")

var templates = map[string]*template.Template{
	filepath.Join("src", "main", "AndroidManifest.xml"): template.Must(template.New("").Parse(`<?xml version="1.0" encoding="utf-8"?>
<manifest
    package="{{.AndroidPackage}}"
    xmlns:android="http://schemas.android.com/apk/res/android"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    xsi:schemaLocation="http://schemas.android.com/apk/res/android ">

    <uses-permission android:name="android.permission.INTERNET"/>

    <uses-sdk android:minSdkVersion="23"/>

    <application >
        <activity android:name="io.v.x.devtools.bendroid.BendroidActivity">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
`)),

	"build.gradle.tmp": template.Must(template.New("build.gradle.tmp").Parse(`
task wrapper(type: Wrapper) {
    gradleVersion = '2.10'
}
`)),

	"build.gradle": template.Must(template.New("build.gradle").Parse(`
buildscript {
    repositories {
        jcenter()
    }

    dependencies {
        classpath 'com.android.tools.build:gradle:1.5.0'
        classpath 'com.jakewharton.sdkmanager:gradle-plugin:0.12.0'
    }
}

repositories {
    mavenCentral()
}

// Both plugins come from mavenCentral.
apply plugin: 'android-sdk-manager'
apply plugin: 'com.android.application'

android {
    buildToolsVersion '23.0.1'
    compileSdkVersion 23

    defaultConfig {
        applicationId "{{.AndroidPackage}}"
        minSdkVersion 23
        targetSdkVersion 23
        versionCode 1
        versionName "1.0"
    }

    sourceSets {
        main {
            jniLibs.srcDir 'src/main/jniLibs/armeabi-v7a'
        }
    }
    signingConfigs {
        release {
            storeFile file("bendroid.keystore")
            storePassword "bendroid"
            keyAlias "bendroid"
            keyPassword "bendroid"
        }
    }
    buildTypes {
        release {
            signingConfig signingConfigs.release
        }
    }
}

dependencies {
    compile 'com.android.support:appcompat-v7:23.1.0'
}
`)),

	javaSrcFile: template.Must(template.New("TestRunner.java").Parse(`
package io.v.x.devtools.bendroid;

import android.app.Activity;
import android.os.Bundle;

public class BendroidActivity extends Activity {
    private native void nativeRun();
    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
    }
    @Override
    public void onStart() {
        super.onStart();
        System.loadLibrary("{{.MainPkg}}");
        nativeRun();
    }
}
`)),

	"main.go": template.Must(template.New("main").Parse(`
package main

// #cgo LDFLAGS: -llog
//
// #include <stdlib.h>
// #include <android/log.h>
// #include <jni.h>
import "C"

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"testing"
	"unsafe"
	{{range .FuncImports}}
	"{{.}}"{{end}}
)

var ctag = C.CString("Bendroid")

type infoWriter struct{}

func (infoWriter) Write(p []byte) (n int, err error) {
	cstr := C.CString(string(p))
	C.__android_log_write(C.ANDROID_LOG_INFO, ctag, cstr)
	C.free(unsafe.Pointer(cstr))
	return len(p), nil
}

func lineLog(f *os.File, priority C.int) {
	const logSize = 1024 // matches android/log.h.
	r := bufio.NewReaderSize(f, logSize)
	for {
		line, _, err := r.ReadLine()
		str := string(line)
		if err != nil {
			str += " " + err.Error()
		}
		cstr := C.CString(str)
		C.__android_log_write(priority, ctag, cstr)
		C.free(unsafe.Pointer(cstr))
		if err != nil {
			break
		}
	}
}

func init() {
	log.SetOutput(infoWriter{})
	// android logcat includes all of log.LstdFlags
	log.SetFlags(log.Flags() &^ log.LstdFlags)

	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stderr = w
	go lineLog(r, C.ANDROID_LOG_ERROR)

	r, w, err = os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout = w
	go lineLog(r, C.ANDROID_LOG_INFO)
}

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

//export Java_io_v_x_devtools_bendroid_BendroidActivity_nativeRun
func Java_io_v_x_devtools_bendroid_BendroidActivity_nativeRun(jenv *C.JNIEnv, jVClass C.jclass) {
	fmt.Fprintf(os.Stderr, "BENDROIDPID=%d\n", os.Getpid())
	// TODO(mattr): Consider using a file to send flags to android instead of compiling
	// them into the apk.
	if len(os.Args) > 1 {
		os.Args = os.Args[:1]
	}
	{{range .Flags}}
	os.Args = append(os.Args, "{{.}}"){{end}}

	describeDevice()
	m := testing.MainStart(regexp.MatchString, tests, benchmarks, examples)
	if testMain == nil {
		os.Exit(m.Run())
	} else {
		testMain(m)
	}
}

func describeDevice() {
	f, err := os.Open("/system/build.prop")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read OS and CPU information: %v", err)
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	var osver struct {
		Release, SDK, BuildID, Incremental string
	}
	var cpu struct {
		Architecture       string
		Brand, Model, Name string
	}
	buildprops := []struct {
		key   []byte
		field *string
	}{
		{[]byte("ro.build.version.release="), &osver.Release},
		{[]byte("ro.build.version.sdk="), &osver.SDK},
		{[]byte("ro.build.id="), &osver.BuildID},
		{[]byte("ro.build.version.incremental="), &osver.Incremental},
		{[]byte("ro.product.cpu.abilist="), &cpu.Architecture},
		{[]byte("ro.product.brand="), &cpu.Brand},
		{[]byte("ro.product.model="), &cpu.Model},
		{[]byte("ro.product.name="), &cpu.Name},
	}
	for s.Scan() {
		byts := s.Bytes()
		for _, p := range buildprops {
			if bytes.HasPrefix(byts, p.key) {
				*p.field = string(bytes.TrimPrefix(byts, p.key))
			}
		}
	}
	fmt.Fprintf(os.Stdout, "BENDROIDCPU_ARCHITECTURE=%v\n", cpu.Architecture)
	fmt.Fprintf(os.Stdout, "BENDROIDCPU_DESCRIPTION=%v\n",
		strings.Join([]string{cpu.Brand, cpu.Model, cpu.Name}, " "))
	fmt.Fprintf(os.Stdout, "BENDROIDOS_VERSION=%v (Build %v Release %v SDK %v)\n",
		osver.Release, osver.BuildID, osver.Incremental, osver.SDK)
}

func main() {
}

`)),
}

var keystore []byte

func init() {
	// Here we add the keystore file.  This is used to sign apks, which
	// is required before an app can be installed.
	// Generated by:
	// keytool -genkey -v -keystore /tmp/keystore -alias bendroid -keyalg RSA \
	// -keysize 2048 -validity 10000 -storepass bendroid -keypass bendroid \
	// -dname CN=bendroid && base64 /tmp/keystore
	data := rmSpace(`
	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`)
	var err error
	keystore, err = base64.StdEncoding.DecodeString(data)
	if err != nil {
		panic(fmt.Sprintf("Bendroid keystore data corrupt: %v", err))
	}
}

func rmSpace(in string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, in)
}
