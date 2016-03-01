// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"sort"
	"strings"

	"v.io/jiri"
)

type cgoHeader struct {
	// The path to the header file
	path              string
	prologue          []string
	pkg               string
	srcGoFilePath     string
	sysIncludes       []string
	typedefs          []string
	exportedFunctions []string
}

const (
	stateBase                 = "base"
	stateInPreamble           = "inPreamble"
	stateInPrologue           = "inPrologue"
	stateInPrologueIntSection = "inPrologueIntSection"
	stateInCppGuardStart      = "inCppGuardStart"
	stateInExports            = "inExporst"
	stateInCppGuardEnd        = "inCppGuardEnd"
	stateDone                 = "done"

	guardedGoIntDeclaration = `#ifdef __LP64__
// 64-bit code
typedef GoInt64 GoInt;
typedef GoUint64 GoUint;
typedef char _check_for_64_bit_pointer_matching_GoInt[sizeof(void*)==64/8 ? 1:-1];
#else
// 32-bit code
typedef GoInt32 GoInt;
typedef GoUint32 GoUint;
typedef char _check_for_32_bit_pointer_matching_GoInt[sizeof(void*)==32/8 ? 1:-1];
#endif`
)

// newCgoHeader returns cgoHeader struct parsed from a given file path
func newCgoHeader(jirix *jiri.X, path string) (*cgoHeader, error) {
	hdr := &cgoHeader{}
	if err := hdr.parseFromFile(jirix, path); err != nil {
		return nil, err
	}
	return hdr, nil
}

func (hdr *cgoHeader) parseFromFile(jirix *jiri.X, path string) error {
	verbose(jirix, "Parsing header file %v\n", path)
	hdr.path = path
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	// Configure FSM base state
	state := stateBase
	handlers := map[string]func(string, *cgoHeader) (string, error){
		stateBase:                 parseBase,
		stateInPreamble:           parseInPreamble,
		stateInPrologue:           parseInPrologue,
		stateInPrologueIntSection: parseInPrologueIntSection,
		stateInCppGuardStart:      parseInCppGuardStart,
		stateInExports:            parseInExports,
		stateInCppGuardEnd:        parseInCppGuardEnd,
		stateDone:                 parseInDone,
	}
	for _, line := range strings.Split(string(bytes), "\n") {
		handler, ok := handlers[state]
		if !ok {
			panic(fmt.Sprintf("Unhandled state: %v", state))
		}
		if cleanedLine := strings.TrimSpace(line); cleanedLine != "" {
			if state, err = handler(cleanedLine, hdr); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseBase(line string, hdr *cgoHeader) (nextState string, err error) {
	nextState = stateBase // Default is same state.
	switch {
	case strings.HasPrefix(line, "/* Created"):
		/* Created by "go tool cgo" - DO NOT EDIT. */
		// Ignore
	case strings.HasPrefix(line, "/* package"):
		/* package v.io/x/swift/impl/google/rt */
		hdr.pkg = extractFromRegex(line, ".*package ([^ ]+).*")
	case strings.Contains(line, "Start of preamble"):
		nextState = stateInPreamble
	case strings.Contains(line, "Start of boilerplate cgo prologue"):
		nextState = stateInPrologue
	case strings.Contains(line, "#ifdef __cplusplus"):
		nextState = stateInCppGuardStart
	}
	return nextState, nil
}

func parseInPreamble(line string, hdr *cgoHeader) (nextState string, err error) {
	nextState = stateInPreamble // Default is same state.
	switch {
	case strings.Contains(line, "End of preamble"):
		//	/* End of preamble from import "C" comments.  */
		nextState = stateBase
	case strings.HasPrefix(line, "#line"):
		// #line 19 "/Users/zinman/vanadium/release/go/src/v.io/x/swift/impl/google/rt/swift.go"
		// ignore
	case strings.HasPrefix(line, "#import"):
		// #import "../../../types.h"
		// ignore
	case strings.HasPrefix(line, "#include"):
		// #include <string.h> // memcpy
		hdr.sysIncludes = append(hdr.sysIncludes, line)
	case strings.HasPrefix(line, "//"):
		// // These sizes (including C struct memory alignment/padding) isn't available from Go, so we make that available via CGo.
		// ignore
	default:
		// static const size_t sizeofSwiftByteArray = sizeof(SwiftByteArray);
		hdr.typedefs = append(hdr.typedefs, line)
	}
	return nextState, nil
}

func parseInPrologue(line string, hdr *cgoHeader) (nextState string, err error) {
	nextState = stateInPrologue // Default is same state.
	switch {
	case strings.HasSuffix(line, " GoInt;"):
		// typedef GoInt64 GoInt; "
		// Add our 32/64-bit clean version instead
		hdr.prologue = append(hdr.prologue, strings.Split(guardedGoIntDeclaration, "\n")...)
		nextState = stateInPrologueIntSection
	case strings.Contains(line, "End of boilerplate cgo prologue"):
		nextState = stateBase
	case strings.HasPrefix(line, "//"):
		// static assertion to make sure the file is being used on architecture
		// at least with matching size of GoInt.
		// (ignore comments in prologue)
	case strings.Contains(line, "_check_for") && strings.Contains(line, "pointer_matching_GoInt"):
		// typedef char _check_for_64_bit_pointer_matching_GoInt[sizeof(void*)==64/8 ? 1:-1];
		// ignore 32/64 bit check as we add our own in our guardedGoIntDeclaration
	default:
		hdr.prologue = append(hdr.prologue, line)
	}
	return nextState, nil
}

func parseInPrologueIntSection(line string, hdr *cgoHeader) (nextState string, err error) {
	nextState = stateInPrologueIntSection // Default is same state.
	switch {
	case strings.HasSuffix(line, " GoUint;"):
		// typedef GoUint64 GoUint;
		nextState = stateInPrologue
	}
	return nextState, nil
}

func parseInCppGuardStart(line string, hdr *cgoHeader) (nextState string, err error) {
	nextState = stateInCppGuardStart // Default is same state.
	switch {
	case line == "#endif":
		nextState = stateInExports
	}
	return nextState, nil

}

func parseInExports(line string, hdr *cgoHeader) (nextState string, err error) {
	nextState = stateInExports // Default is same state.
	switch {
	case line == "#ifdef __cplusplus":
		nextState = stateInCppGuardEnd
	default:
		hdr.exportedFunctions = append(hdr.exportedFunctions, line)
	}
	return nextState, nil

}

func parseInCppGuardEnd(line string, hdr *cgoHeader) (nextState string, err error) {
	nextState = stateInCppGuardEnd // Default is same state.
	switch {
	case line == "#endif":
		nextState = stateBase
	}
	return nextState, nil
}

func parseInDone(line string, hdr *cgoHeader) (nextState string, err error) {
	return "", fmt.Errorf("Unexpected string when in state done: %v", line)
}

// Helpers for merging sections across a collection of parsed headers

type cgoHeaders []*cgoHeader

func (hdrs cgoHeaders) includes() []string {
	includes := hdrs.dedupedStrings(func(hdr *cgoHeader) []string {
		return hdr.sysIncludes
	})
	sort.Strings(includes)
	return includes
}

func (hdrs cgoHeaders) typedefs() []string {
	typedefs := hdrs.dedupedStrings(func(hdr *cgoHeader) []string {
		return hdr.typedefs
	})
	sort.Strings(typedefs)
	return typedefs
}

func (hdrs cgoHeaders) exports() []string {
	exports := []string{}
	for _, hdr := range hdrs {
		exports = append(exports, "/* package "+hdr.pkg+" */")
		exports = append(exports, hdr.exportedFunctions...)
	}
	return exports
}

func (hdrs cgoHeaders) dedupedStrings(itemsCallback func(hdr *cgoHeader) []string) []string {
	deduped := []string{}
	seen := map[string]bool{}
	for _, collectionItem := range hdrs {
		strs := itemsCallback(collectionItem)
		for _, str := range strs {
			if seen[str] {
				// ignore
			} else {
				deduped = append(deduped, str)
				seen[str] = true
			}
		}
	}
	return deduped
}
