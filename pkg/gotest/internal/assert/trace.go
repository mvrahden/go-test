package assert

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

const gotestDirMarker = "/pkg/gotest/"

// CallerTrace walks the call stack and returns a "called from: file:line"
// annotation pointing to the user's call site.
//
// It skips gotest-internal frames (assertion mechanics), collects user
// frames, and stops at the first boundary sentinel (execTestFn from
// T.It/T.When, or ƒƒ_GOTEST_exec from the generated bridge).
func CallerTrace() string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs) // skip runtime.Callers + CallerTrace
	if n == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pcs[:n])
	var userFrames []runtime.Frame
	for {
		frame, more := frames.Next()

		if isBoundary(frame.Function) {
			break
		}
		if strings.HasPrefix(frame.Function, "testing.") ||
			strings.HasPrefix(frame.Function, "runtime.") {
			break
		}
		if isGotestSource(frame.File) || isGeneratedBridge(frame.File) {
			if !more {
				break
			}
			continue
		}

		userFrames = append(userFrames, frame)
		if !more {
			break
		}
	}

	if len(userFrames) == 0 {
		return ""
	}

	f := userFrames[len(userFrames)-1]
	return fmt.Sprintf("\n  called from: %s:%d", filepath.Base(f.File), f.Line)
}

func isBoundary(fn string) bool {
	return strings.HasSuffix(fn, ".execTestFn") ||
		strings.HasSuffix(fn, ".ƒƒ_GOTEST_exec")
}

func isGotestSource(file string) bool {
	return !strings.HasSuffix(file, "_test.go") &&
		strings.Contains(file, gotestDirMarker)
}

func isGeneratedBridge(file string) bool {
	return strings.HasPrefix(filepath.Base(file), "ƒƒ_")
}
