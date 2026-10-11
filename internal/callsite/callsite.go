// Package callsite names source locations the way go test prints them.
package callsite

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

// IsStd reports whether a frame of function fn in file belongs to the
// standard library: a frame no user wrote.
func IsStd(fn, file string) bool {
	if src := stdSrc(); src != "" {
		return strings.HasPrefix(filepath.ToSlash(file), src)
	}
	// -trimpath leaves no GOROOT to compare: judge by import path, whose
	// first element has a dot unless it is the standard library's.
	return stdPath(fn)
}

// stdSrc is GOROOT/src/ as frames name it, empty under -trimpath.
var stdSrc = sync.OnceValue(func() string {
	pc := reflect.ValueOf(fmt.Sprint).Pointer()
	file, _ := runtime.FuncForPC(pc).FileLine(pc)
	src, ok := strings.CutSuffix(filepath.ToSlash(file), "fmt/print.go")
	if !ok || !strings.Contains(src, "/") {
		return ""
	}
	return src
})

func stdPath(fn string) bool {
	slash := max(strings.LastIndexByte(fn, '/'), 0)
	pkg := fn
	if dot := strings.IndexByte(fn[slash:], '.'); dot >= 0 {
		pkg = fn[:slash+dot]
	}
	first, _, _ := strings.Cut(pkg, "/")
	return pkg != "main" && !strings.Contains(first, ".")
}
