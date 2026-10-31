// Package genstub stands a bare package clause in for every generated file
// on disk while packages load. One left by an earlier generate may name
// suites since renamed; the harness is generated anew anyway, so the file is never read as written.
package genstub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/pkgpattern"
)

// Stubs maps each generated file on disk in the packages the patterns match
// to its package clause. dir is where go runs, "" the current directory.
// Best effort: a listing that fails stubs nothing.
func Stubs(dir string, patterns, buildFlags []string) map[string][]byte {
	args := append([]string{"list", "-e", "-find", "-f", "{{.Dir}}"}, buildFlags...)
	cmd := exec.Command("go", append(args, pkgpattern.Clean(patterns)...)...) //nolint:gosec // G204: the go tool with the command's own patterns
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	_ = cmd.Run()
	return StubsIn(strings.Fields(stdout.String())...)
}

// StubsIn is Stubs for package directories already known.
func StubsIn(dirs ...string) map[string][]byte {
	stubs := map[string][]byte{}
	for _, d := range dirs {
		for _, name := range about.GeneratedFileNames {
			path := filepath.Join(d, name)
			src, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if stub := about.PackageStub(src); stub != nil {
				stubs[path] = stub
			}
		}
	}
	return stubs
}

// LoadFlags is buildFlags with a go -overlay that stubs the generated files
// in the packages the patterns match, for a load to take alongside
// about.ParseStubbingGenerated: the go command then reads the stubs too.
// Without generated files, or beside an -overlay of the caller's own (go
// takes one), the flags come back as they are.
func LoadFlags(dir string, patterns, buildFlags []string) (flags []string, cleanup func(), err error) {
	for _, f := range buildFlags {
		if f == "-overlay" || strings.HasPrefix(f, "-overlay=") {
			return buildFlags, func() {}, nil
		}
	}
	stubs := Stubs(dir, patterns, buildFlags)
	if len(stubs) == 0 {
		return buildFlags, func() {}, nil
	}
	flag, cleanup, err := WriteOverlay(os.Getenv("GOTMPDIR"), stubs)
	if err != nil {
		return nil, nil, err
	}
	return append(append([]string{}, buildFlags...), flag), cleanup, nil
}

// WriteOverlay writes files as a go -overlay under tmp ("" the system
// temporary directory) and returns the flag naming it.
func WriteOverlay(tmp string, files map[string][]byte) (flag string, cleanup func(), err error) {
	dir, err := os.MkdirTemp(tmp, "gotest-stub-overlay")
	if err != nil {
		return "", nil, fmt.Errorf("write overlay: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	replace := make(map[string]string, len(files))
	i := 0
	for file, src := range files {
		i++
		path := filepath.Join(dir, fmt.Sprintf("%d-%s", i, filepath.Base(file)))
		if err := os.WriteFile(path, src, 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("write overlay: %w", err)
		}
		replace[file] = path
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "overlay.json"), data, 0o600)
	}
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write overlay: %w", err)
	}
	return "-overlay=" + filepath.Join(dir, "overlay.json"), cleanup, nil
}
