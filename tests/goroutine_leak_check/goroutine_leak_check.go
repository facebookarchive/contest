// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package goroutine_leak_check

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

var (
	goidRegex     = regexp.MustCompile(`^\S+\s+(\d+)`)
	funcNameRegex = regexp.MustCompile(`^(\S+)\(\S+`)
	fileLineRegex = regexp.MustCompile(`^\s*(\S+):(\d+)`)
)

type stackTraceEntry struct {
	GoID  int
	File  string
	Line  int
	Func  string
	Trace []string
}

func (e *stackTraceEntry) String() string {
	return fmt.Sprintf("%s:%d %s %d", e.File, e.Line, e.Func, e.GoID)
}

// CheckLeakedGoRoutines is used to check for goroutine leaks at the end of a test.
// It is not uncommon to leave a go routine that will never finish,
// e.g. blocked on a channel that is unreachable will never be closed.
// This function enumerates goroutines and reports any goroutines that are left running.
func CheckLeakedGoRoutines(funcWhitelist ...string) error {
	// Get goroutine stacks
	buf := make([]byte, 1000000)
	n := runtime.Stack(buf, true /* all */)
	// First goroutine is always the running one, we skip it.
	ph := 0
	var e *stackTraceEntry
	var badEntries []string
	for _, line := range strings.Split(string(buf[:n]), "\n") {
		switch ph {
		case 0: // Look for an empty line
			if len(line) == 0 {
				if e != nil {
					found := false
					for _, wle := range funcWhitelist {
						if wle == e.Func {
							found = true
							break
						}
					}
					if !found {
						badEntries = append(badEntries, e.String())
					}
				}
				e = &stackTraceEntry{}
				ph++
			} else {
				if e != nil {
					e.Trace = append(e.Trace, line)
				}
			}
		case 1: // Extract goroutine id
			if m := goidRegex.FindStringSubmatch(line); m != nil {
				if goid, err := strconv.Atoi(m[1]); err == nil {
					e.GoID = goid
					ph++
					break
				}
			}
			panic(fmt.Sprintf("Cannot parse backtrace (goid) %q", line))
		case 2: // Extract function name.
			e.Trace = append(e.Trace, line)
			if m := funcNameRegex.FindStringSubmatch(line); m != nil {
				e.Func = m[1]
				ph++
				break
			}
			if e.Func != "" {
				// This means entire routine is in stdlib, ignore it.
				ph = 1
				break
			}
			panic(fmt.Sprintf("Cannot parse backtrace (func) %q", line))
		case 3: // Extract file name
			e.Trace = append(e.Trace, line)
			if m := fileLineRegex.FindStringSubmatch(line); m != nil {
				e.File = filepath.Base(m[1])
				if ln, err := strconv.Atoi(m[2]); err == nil {
					e.Line = ln
				}
				// We are looking for a non-stdlib function.
				if !strings.Contains(e.Func, "/") || !strings.Contains(path.Dir(e.Func), ".") {
					ph = 2
				} else {
					ph = 0
				}
				break
			}
			panic(fmt.Sprintf("Cannot parse backtrace (file) %q", line))
		}
	}
	if len(badEntries) > 0 {
		sort.Strings(badEntries)
		return fmt.Errorf("leaked goroutines:\n  %s\n", strings.Join(badEntries, "\n  "))
	}
	return nil
}
