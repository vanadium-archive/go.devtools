package version

// Version identifies a version of a tool. Automated builds should set
// this value to something meaningful during the build as follows:
//
// go build -ldflags "-X veyron.io/tools/lib/version.Version <version>" veyron.io/tools/<tool>
var Version string = "manual-build"
