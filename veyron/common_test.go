package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"veyron.io/tools/lib/util"
)

func createConfig(t *testing.T, ctx *util.Context, config *util.CommonConfig) {
	configFile, err := util.ConfigFile("common")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Run().MkdirAll(filepath.Dir(configFile), os.FileMode(0755)); err != nil {
		t.Fatalf("%v", err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Run().WriteFile(configFile, data, os.FileMode(0644)); err != nil {
		t.Fatalf("WriteFile(%v) failed: %v", configFile, err)
	}
}
