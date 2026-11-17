package assert

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/mvrahden/go-test/internal/callsite"
)

// gotestRoots are the directories of gotest's own runtime code, the
// assertion library and the harness that runs suite methods, as this file's
// frames name them: a user's directory of the same name is not one. Nil when
// the path of this file cannot be cut.
var gotestRoots = sync.OnceValue(func() []string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil
	}
	root, cut := strings.CutSuffix(filepath.ToSlash(file), "pkg/gotest/internal/assert/trace.go")
	if !cut || root == "" {
		return nil
	}
	return []string{root + "pkg/gotest/", root + "pkg/gotestruntime/"}
})

// gotestDirMarkers name the same directories by their last elements, for
// when gotestRoots are unknown.
var gotestDirMarkers = []string{"/pkg/gotest/", "/pkg/gotestruntime/"}

// CallerFrame returns the user's call site as "file:line",
// or "" if no user frame is found.
func CallerFrame() string {
	f := callerFrame(2)
	if f == nil {
		return ""
	}
	return callsite.Format(f.File, f.Line)
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
		// Neither gotest nor the standard library is the user's code: a fuzz
		// body is called through reflect.
		if IsGotestSource(frame.File) || IsGeneratedBridge(frame.File) || callsite.IsStd(frame.Function, frame.File) {
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

// IsGotestSource reports whether file, not a test file, is gotest's own:
// under gotest's pkg/gotest or pkg/gotestruntime, not a directory of the
// user's named like them. Frames from there are machinery, never the user's
// call site.
func IsGotestSource(file string) bool {
	if strings.HasSuffix(file, "_test.go") {
		return false
	}
	if roots := gotestRoots(); roots != nil {
		file = filepath.ToSlash(file)
		for _, root := range roots {
			if strings.HasPrefix(file, root) {
				return true
			}
		}
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
