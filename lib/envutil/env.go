// Package envutil implements utilities for processing environment variables.
// There are three representations of environment variables:
//    1) map[key]value   # used by veyron libraries
//    2) []`key=value`   # used by standard Go packages
//    3) []`key="value"` # used by shells for setting the environment
//
// The map form (1) is used by veyron since it's more convenient for read,
// modification, and write of individual variables.  The slice form (2) is used
// by standard Go packages, presumably since it's similar to the underlying os
// implementation.  The slice form (3) is used by shells, which need the
// appropriate quoting on the command line.
package envutil

import (
	"sort"
	"strings"
)

// ToMap converts environment variables from the []`key=value` form to the
// map[key]value form.  This is the representation used by veyron libraries.
func ToMap(env []string) map[string]string {
	ret := make(map[string]string, len(env))
	for _, entry := range env {
		if entry == "" {
			continue
		}
		switch kv := strings.SplitN(entry, "=", 2); len(kv) {
		case 2:
			ret[kv[0]] = kv[1]
		default:
			ret[kv[0]] = ""
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// ToSlice converts environment variables from the map[key]value form to the
// []`key=value` form.  This is the representation used by standard Go packages.
// The result is sorted.
func ToSlice(env map[string]string) []string {
	return toSlice(env, false)
}

// ToQuotedSlice converts environment variables from the map[key]value form to
// the []`key="value"` form.  This is the representation used by shells for
// setting the environment; the value is surrounded by double-quotes, and
// double-quotes within the value are escaped.  The result is sorted.
func ToQuotedSlice(env map[string]string) []string {
	return toSlice(env, true)
}

func toSlice(env map[string]string, quote bool) []string {
	ret := make([]string, 0, len(env))
	for key, val := range env {
		if key == "" {
			continue
		}
		if quote {
			val = quoteForShell(val)
		}
		ret = append(ret, key+"="+val)
	}
	sort.Strings(ret)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func quoteForShell(s string) string {
	return `"` + strings.Replace(s, `"`, `\"`, -1) + `"`
}

// Replace inserts (key,value) pairs from src into dst.  If a key in src already
// exists in dst, the dst value is overwritten with the src value.
func Replace(dst, src map[string]string) {
	for key, val := range src {
		dst[key] = val
	}
}
