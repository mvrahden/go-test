package gotestrunner

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
)

// buildLog is what one go test -c -json call said, by package. go prints
// a build-output event per line of a package's report and a build-fail
// event when its build failed, each with the ImportPath it is about;
// "# pkg" headers are not read, since vet prints two and a dependency that
// fails prints one for all its dependents.
type buildLog struct {
	mu      sync.Mutex
	group   map[string]bool
	partial []byte
	output  map[string][]byte
	fail    map[string]bool
	other   []byte
}

func newBuildLog(group []string) *buildLog {
	l := &buildLog{group: map[string]bool{}, output: map[string][]byte{}, fail: map[string]bool{}}
	for _, pkg := range group {
		l.group[pkg] = true
	}
	return l
}

// Write takes go's stdout; it is safe to call while failed and diagnostics
// are read. A line is read once it is whole.
func (l *buildLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial = append(l.partial, p...)
	for {
		i := bytes.IndexByte(l.partial, '\n')
		if i < 0 {
			break
		}
		l.line(l.partial[:i+1])
		l.partial = l.partial[i+1:]
	}
	return len(p), nil
}

// line books one whole line, newline included.
func (l *buildLog) line(line []byte) {
	var e struct {
		ImportPath, Package, Action, Output string
	}
	if err := json.Unmarshal(line, &e); err != nil {
		l.other = append(l.other, line...)
		return
	}
	switch {
	case e.ImportPath == "" && e.Package != "":
		// A test2json frame of a setup failure; the build events say it.
	case e.ImportPath == "":
		l.other = append(l.other, e.Output...)
	default:
		pkg := eventPackage(e.ImportPath)
		if !l.group[pkg] {
			l.other = append(l.other, e.Output...)
			return
		}
		switch e.Action {
		case "build-output":
			l.output[pkg] = append(l.output[pkg], e.Output...)
		case "build-fail":
			l.fail[pkg] = true
		}
	}
}

// failed reports whether a build-fail event named pkg.
func (l *buildLog) failed(pkg string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fail[pkg]
}

// diagnostics is pkg's build output, in order.
func (l *buildLog) diagnostics(pkg string) []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bytes.Clone(l.output[pkg])
}

// shared is the output of packages outside the group and the lines that
// are not events.
func (l *buildLog) shared() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bytes.Clone(l.other)
}

// eventPackage is the package an event's ImportPath belongs to: P for
// anything built for P's test ("X [P.test]"), else the path itself.
func eventPackage(importPath string) string {
	if strings.HasSuffix(importPath, ".test]") {
		if i := strings.LastIndex(importPath, " ["); i >= 0 {
			return strings.TrimSuffix(importPath[i+2:], ".test]")
		}
	}
	return importPath
}
