package assert

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// gotestDirMarkers name the directories of gotest's own runtime code: the
// assertion library and the harness that runs suite methods. Frames from
// either are machinery, never the user's call site.
var gotestDirMarkers = []string{"/pkg/gotest/", "/pkg/gotestruntime/"}

// CallerFrame returns the user's call site as "file:line",
// or "" if no user frame is found.
func CallerFrame() string {
	f := callerFrame(2)
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", filepath.Base(f.File), f.Line)
}

// callerFrame resolves the outermost user frame on the call stack.
// skip is passed to runtime.Callers (caller should pass 2 to skip
// runtime.Callers + callerFrame itself; public wrappers add +1 for
// themselves via the extra skip=2 they pass).
func callerFrame(skip int) *runtime.Frame {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(skip+1, pcs) // +1 to also skip callerFrame
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	var userFrames []runtime.Frame
	for {
		frame, more := frames.Next()

		if IsBoundary(frame.Function) {
			break
		}
		if strings.HasPrefix(frame.Function, "testing.") ||
			strings.HasPrefix(frame.Function, "runtime.") {
			break
		}
		if IsGotestSource(frame.File) || IsGeneratedBridge(frame.File) {
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
		return nil
	}

	f := userFrames[len(userFrames)-1]
	return &f
}

func IsBoundary(fn string) bool {
	return strings.HasSuffix(fn, ".execTestFn") ||
		strings.HasSuffix(fn, ".ƒƒ_GOTEST_exec")
}

func IsGotestSource(file string) bool {
	if strings.HasSuffix(file, "_test.go") {
		return false
	}
	for _, marker := range gotestDirMarkers {
		if strings.Contains(file, marker) {
			return true
		}
	}
	return false
}

// IsGeneratedBridge reports whether file is a generated suite harness, with
// or without the generated-file prefix.
func IsGeneratedBridge(file string) bool {
	name := filepath.Base(file)
	return strings.HasSuffix(name, "gotest_psuite_test.go") || strings.HasSuffix(name, "gotest_pxsuite_test.go")
}
