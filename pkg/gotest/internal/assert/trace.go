package assert

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

const gotestDirMarker = "/pkg/gotest/"

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
	return !strings.HasSuffix(file, "_test.go") &&
		strings.Contains(file, gotestDirMarker)
}

func IsGeneratedBridge(file string) bool {
	name := filepath.Base(file)
	return name == "gotest_psuite_test.go" || name == "gotest_pxsuite_test.go"
}
