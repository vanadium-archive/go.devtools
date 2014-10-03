package util

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	archRE = regexp.MustCompile("amd64|arm|x86")
)

// Platform describe a hardware and software platform.
type Platform struct {
	// Arch is the platform architecture (e.g. arm or amd64).
	Arch string
	// Sub is the platform sub-architecture (e.g. v6 or v7 for arm)
	Sub string
	// Sys is the platform operating system (e.g. linux or darwin)
	Sys string
}

// ParsePlatform parses a string in the format <arch><sub>-<sys> to a
// Platform.
func ParsePlatform(platform string) (*Platform, error) {
	result := &Platform{}
	tokens := strings.Split(platform, "-")
	if expected, got := 2, len(tokens); expected != got {
		return nil, fmt.Errorf("invalid length of %v: expected %v, got %v", tokens, expected, got)
	}
	result.Arch, result.Sub = parseArch(tokens[0])
	result.Sys = tokens[1]
	return result, nil
}

// parseArch parses a string of the format <arch><sub> into a tuple
// <arch>, <sub>.
func parseArch(arch string) (string, string) {
	if loc := archRE.FindStringIndex(arch); loc != nil && loc[0] == 0 {
		return arch[loc[0]:loc[1]], arch[loc[1]:]
	} else {
		return "unknown", "unknown"
	}
}
