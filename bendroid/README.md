# bendroid: Benchmark and run tests on Android

## Prerequisites & installation

- [Android Studio](http://developer.android.com/sdk/index.html) (which should make `adb` and `gradle` available in your path)
- [Android NDK Toolchain](http://developer.android.com/ndk/index.html)
- [Go](https://golang.org) version 1.6 or greater
- To install this tool: `go get v.io/x/devtools/bendroid`


## [Usage](https://godoc.org/v.io/x/devtools/bendroid)
```
export NDK_TOOLCHAIN=<path where the NDK toolchain has been unpacked>
CC=${NDK_TOOLCHAIN}/arm-21/bin/arm-linux-androideabi-gcc \
CXX=${NDK_TOOLCHAIN}/arm-21/bin/arm-linux-androideabi-g++ \
${GOPATH}/bin/bendroid --help
```

## [Vanadium contributors](http://vanadium.github.io/community/contributing.html)

All the above requirements are taken care of by the `android` profile.

```
jiri v23-profile install android
jiri go install v.io/x/devtools/bendroid
jiri run --target arm-android ${JIRI_ROOT}/release/go/bin/bendroid --help
```

