package version

// Version identifies a version of a tool. Automated builds should set
// this value to something meaningful during the build as follows:
//
// go build -ldflags "-X tools/lib/version.Version <version>" tools/git-veyron
var Version string = "manual-build"
