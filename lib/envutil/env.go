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
	"fmt"
	"os"
	"os/exec"
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

// Copy returns a copy of m.  The returned map is never nil.
func Copy(env map[string]string) map[string]string {
	envCopy := make(map[string]string, len(env))
	for key, val := range env {
		envCopy[key] = val
	}
	return envCopy
}

// Replace inserts (key,value) pairs from src into dst.  If a key in src already
// exists in dst, the dst value is replaced with the src value.
func Replace(dst, src map[string]string) {
	for key, val := range src {
		dst[key] = val
	}
}

// Snapshot manages a mutable snapshot of environment variables.
//
// Snapshot is initialized with a base environment, and may be mutated with
// calls to Set or SetTokens.  The resulting environment is retrieved with calls
// to Map or Slice.
//
// Mutations are tracked separately from the base environment; call DeltaMap to
// retrieve only the environment variables that have changed.
type Snapshot struct {
	base, delta map[string]string
}

// NewSnapshot returns a new Snapshot with the given base environment.  The base
// is copied so that the snapshot will ignore subsequent changes to base.
func NewSnapshot(base map[string]string) *Snapshot {
	return &Snapshot{Copy(base), make(map[string]string)}
}

// NewSnapshotFromOS returns a new Snapshot with the base environment from
// os.Environ.
func NewSnapshotFromOS() *Snapshot {
	return NewSnapshot(ToMap(os.Environ()))
}

// Get returns the value for the given key.
func (s *Snapshot) Get(key string) string {
	if val, ok := s.delta[key]; ok {
		return val
	}
	return s.base[key]
}

// GetTokens tokenizes the value for the given key with the given separator,
// dropping empty tokens.
func (s *Snapshot) GetTokens(key, separator string) []string {
	var result []string
	for _, token := range strings.Split(s.Get(key), separator) {
		if token != "" {
			result = append(result, token)
		}
	}
	return result
}

// LookPath searches for an executable binary named file in the
// directories named by the PATH environment variable of the snapshot.
//
// NOTE: This function temporarily modifies the global environment and
// therefore is not thread-safe.
func (s *Snapshot) LookPath(file string) (string, error) {
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", s.Get("PATH")); err != nil {
		return "", fmt.Errorf("Setenv(%q, %q) failed: %v", "PATH", s.Get("PATH"), err)
	}
	defer os.Setenv("PATH", oldPath)
	path, err := exec.LookPath(file)
	if err != nil {
		return "", fmt.Errorf("LookPath(%q) failed: %v", file, err)
	}
	return path, nil
}

// Set assigns the value to the given key.
func (s *Snapshot) Set(key, value string) {
	s.delta[key] = value
}

// SetTokens joins the tokens with the given separator, and assigns the
// resulting value to the given key.
func (s *Snapshot) SetTokens(key string, tokens []string, separator string) {
	s.Set(key, strings.Join(tokens, separator))
}

// Map returns a copy of the environment as a map.
func (s *Snapshot) Map() map[string]string {
	dst := Copy(s.base)
	Replace(dst, s.delta)
	return dst
}

// Slice returns a copy of the environment as a slice.
func (s *Snapshot) Slice() []string {
	return ToSlice(s.Map())
}

// BaseMap returns a copy of the original base environment.
func (s *Snapshot) BaseMap() map[string]string {
	return Copy(s.base)
}

// DeltaMap returns a copy of the environment variables that have been mutated.
func (s *Snapshot) DeltaMap() map[string]string {
	return Copy(s.delta)
}
