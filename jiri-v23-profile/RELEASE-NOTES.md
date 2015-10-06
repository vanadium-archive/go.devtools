# Release Notes 9/25/15

This note summarises the state of the currently 'new style' profiles as of 9/25/15.

Please read the docs for v.io/jiri/profiles to understand how the new scheme works
in detail. In outline, profiles are now separate packages, linked into a jiri
subcommand (eg. jiri-xprofile), that can be installed/updated/uninstalled. Profiles
represent source code and are built for specific targets, thus cross compilation
is inherently supported and it's possible to support multiple targets for each profile.
The profiles package maintains a manifest of the currently installed profiles and their
configuration (currently $JIRI_ROOT/.jiri_xprofiles) and the various other jiri
subcommands (e.g. jiri-go, jiri-test etc) read the manifest to configure themselves
appropriately for the required profile and target. For native compilation this
entire mechanism is largely hidden since sensible defaults are used throughout.

Thus, the common usage is:

$ jiri xprofile install base
$ jiri xprofile update base
$
$ jiri go build v.io/...

Cross compilation would be achieved as follows:

$ jiri xprofile install --target=arm-linux base
$ jiri go --target=arm-linux build v.io/...

Native compilation would still work as before:

$ jiri go build v.io/...

## The Profiles

The old style profiles and new style can coexist side-by-side. As of today, all
of the old style profiles have been ported, but not all have been tested. The
tested and in-use ones are:

1. base (syncbase+go)
2. java
3. android

- Cross compilation from linux hosts to linux-arm is not yet fully tested.
- Cross compilation from darwin hosts to linux-arm has yet to be integrated.
- nacl and nodejs profiles are not fully tested yet

## Transition

Both old and new style profiles can live side-by-side. Once a new
style profile is installed, the tools will by default use it unless the
JIRI_PROFILE environment variable is set. This makes it tasy to experiment
with the new setup and yet revert to the old by either setting JIRI_PROFILE
appropriately or moving the manifest file out of the way.


