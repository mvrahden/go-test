// Package callsite names source locations the way go test prints them.
package callsite

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

// Format names file:line as testing does: the base name, or the full path
// under -test.fullpath.
func Format(file string, line int) string {
	if !fullPath() {
		file = filepath.Base(file)
	}
	return file + ":" + strconv.Itoa(line)
}

// Declared turns a declaration the generator recorded as base:line into the
// form [Format] gives. go test runs the binary in the package directory, so
// that is where the file is; slashes as in a frame's file, on Windows too.
func Declared(at string) string {
	if at == "" || !fullPath() {
		return at
	}
	file, line, ok := strings.Cut(at, ":")
	if !ok || filepath.IsAbs(file) {
		return at
	}
	dir, err := os.Getwd()
	if err != nil {
		return at
	}
	return filepath.ToSlash(filepath.Join(dir, file)) + ":" + line
}

func fullPath() bool {
	f := flag.Lookup("test.fullpath")
	return f != nil && f.Value.String() == "true"
}

// IsStd reports whether a frame of function fn in file belongs to the
// standard library: a frame no user wrote.
func IsStd(fn, file string) bool {
	if src := stdSrc(); src != "" {
		return strings.HasPrefix(filepath.ToSlash(file), src)
	}
	// -trimpath leaves no GOROOT to compare: judge by import path. The main
	// module's is known from the build, and may have no dot; any other
	// path's first element has a dot unless it is the standard library's.
	return stdPath(fn, mainModule())
}

// mainModule is the import path of the module the binary was built from, as
// the build recorded it; "" when it did not.
var mainModule = sync.OnceValue(func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Path
	}
	return ""
})

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

// stdPath judges the function fn by its import path: never standard when the
// path is the main module main or under it, otherwise standard when its first
// element has no dot.
func stdPath(fn, main string) bool {
	slash := max(strings.LastIndexByte(fn, '/'), 0)
	pkg := fn
	if dot := strings.IndexByte(fn[slash:], '.'); dot >= 0 {
		pkg = fn[:slash+dot]
	}
	if main != "" && (pkg == main || strings.HasPrefix(pkg, main+"/")) {
		return false
	}
	first, _, _ := strings.Cut(pkg, "/")
	return pkg != "main" && !strings.Contains(first, ".")
}
