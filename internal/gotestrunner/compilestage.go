package gotestrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// errBuildFailed is the error of a package go reported a failed build of:
// the text one go process per package exited with.
var errBuildFailed = errors.New("exit status 1")

// pollInterval is how often a running build's stage dir is looked at.
const pollInterval = 25 * time.Millisecond

// compileGroup builds group in one go call and reports each package once:
// its binary as soon as it is ready, or its failure as go attributed it.
// It returns the packages of a group of several that go left without a
// verdict of their own (a dependency failed), for the caller to build
// alone, where whatever go says is theirs.
func compileGroup(ctx context.Context, group []string, overlayFlag string, buildFlags []string, stageDir, goTmp, cacheDir string, emit func(CompileOutcome)) []string {
	staged, err := stageGroup(group, stageDir, cacheDir)
	if err == nil {
		err = os.MkdirAll(goTmp, 0o755)
	}
	if err != nil {
		for _, pkg := range group {
			emit(CompileOutcome{Package: pkg, Err: compileError(pkg, err, nil)})
		}
		return nil
	}
	args := append([]string{"test", "-c", "-json", overlayFlag, "-o", stageDir + string(os.PathSeparator)}, buildFlags...)
	args = append(args, group...)
	env := append(os.Environ(), "GOTMPDIR="+goTmp)
	what := "test binary for " + group[0]
	if len(group) > 1 {
		what = fmt.Sprintf("test binaries for %d packages", len(group))
	}
	log := newBuildLog(group)
	var stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		defer logSlowBuild(os.Stderr, what, slowBuildThreshold())()
		done <- goTestC(ctx, args, env, log, &stderr)
	}()

	var safe bool
	if info, err := os.Stat(stageDir); err == nil {
		safe = renameSafe(runtime.GOOS, info, goTmp, goCacheDir())
	}
	ready := make([]readiness, len(staged))
	reported := make([]bool, len(staged))
	pass := func(exited int) {
		for i := range staged {
			b := &staged[i]
			if reported[i] || ctx.Err() != nil {
				continue
			}
			if log.failed(b.Package) {
				reported[i] = true
				emit(CompileOutcome{Package: b.Package, Err: compileError(b.Package, errBuildFailed, log.diagnostics(b.Package))})
				continue
			}
			now, _ := os.Stat(b.Path)
			if r := binaryReady(b.placed, now, safe, exited); r != notReady {
				reported[i], ready[i] = true, r
				emit(CompileOutcome{Package: b.Package, Result: CompileResult{Package: b.Package, BinaryPath: b.Path}})
			}
		}
	}
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	var runErr error
wait:
	for {
		select {
		case runErr = <-done:
			break wait
		case <-tick.C:
			pass(-1)
		}
	}
	pass(exitCode(runErr))
	forwardBuildOutput(staged, reported, log, &stderr)

	var left []string
	for i := range staged {
		b := &staged[i]
		switch {
		case reported[i]:
			if ctx.Err() == nil {
				publish(b, ready[i])
			}
		case ctx.Err() != nil:
		case len(group) > 1:
			left = append(left, b.Package)
		default:
			diags := slices.Concat(log.diagnostics(b.Package), log.shared(), stderr.Bytes())
			if runErr == nil {
				runErr = errors.New("go test -c wrote no test binary")
			}
			emit(CompileOutcome{Package: b.Package, Err: compileError(b.Package, runErr, diags)})
		}
	}
	return left
}

// forwardBuildOutput writes to stderr what go said that no failure carries:
// the output of the packages that built (-x traces, toolchain notices), and,
// once every package has its verdict, the rest. A package left without one
// gets the rest in its failure instead.
func forwardBuildOutput(staged []stagedBinary, reported []bool, log *buildLog, stderr *bytes.Buffer) {
	var out []byte
	all := true
	for i := range staged {
		switch {
		case !reported[i]:
			all = false
		case !log.failed(staged[i].Package):
			out = append(out, log.diagnostics(staged[i].Package)...)
		}
	}
	if all {
		out = append(append(out, log.shared()...), stderr.Bytes()...)
	}
	if len(out) > 0 {
		_, _ = os.Stderr.Write(out)
	}
}

// exitCode is go's exit code: 0 for nil, the process's code, else 1.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() > 0 {
		return exit.ExitCode()
	}
	return 1
}

// stagedBinary is one package's slot in a group's stage dir.
type stagedBinary struct {
	Package string
	Path    string      // <stage>/<testBinaryName>
	Cached  string      // the cache entry, "" without a cache
	placed  os.FileInfo // the file put at Path before the build, nil for none
	linked  bool        // placed by a hard link
	damaged bool        // the entry no longer matched its record
}

// stageGroup creates stageDir and places each package's cached binary in
// it under go's name, so go confirms it or replaces it. An entry that
// cannot be locked, or no longer matches its record, is not placed: go
// links that package again.
func stageGroup(group []string, stageDir, cacheDir string) ([]stagedBinary, error) {
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return nil, err
	}
	staged := make([]stagedBinary, len(group))
	for i, pkg := range group {
		b := &staged[i]
		b.Package = pkg
		b.Path = filepath.Join(stageDir, testBinaryName(pkg, runtime.GOOS))
		if cacheDir != "" {
			b.Cached = filepath.Join(cacheDir, sanitizePkgName(pkg)+".test")
			place(b)
		}
	}
	return staged, nil
}

// place puts b's cache entry at its slot, under the entry's lock.
func place(b *stagedBinary) {
	if _, err := os.Stat(b.Cached); err != nil {
		return
	}
	unlock, err := lockFile(b.Cached + ".lock")
	if err != nil {
		return // errLockReplaced included: the entry was evicted
	}
	defer unlock()
	record, recorded := readSum(b.Cached)
	if !recorded {
		return
	}
	if sum, err := sumFile(b.Cached); err != nil || sum != record {
		b.damaged = err == nil
		return
	}
	linked, err := linkOrCopy(b.Cached, b.Path)
	if err != nil {
		_ = os.Remove(b.Path)
		return
	}
	if info, err := os.Stat(b.Path); err == nil {
		b.placed, b.linked = info, linked
	}
}

type readiness int

const (
	notReady  readiness = iota
	confirmed           // the placed file, left by go: up to date
	rebuilt             // a file go wrote
)

// binaryReady judges one slot. exited is go's exit: -1 while it runs. The
// placed file is confirmed only once go exited 0 and left it there: its
// mtime is no sign, since every run's slot links the same cache entry and
// another run's go touches it. A new file is ready once go exited, or as
// soon as it is there where go renames it into place.
func binaryReady(placed, now os.FileInfo, renameSafe bool, exited int) readiness {
	switch {
	case now == nil:
		return notReady
	case placed != nil && os.SameFile(placed, now):
		if exited == 0 {
			return confirmed
		}
		return notReady
	case renameSafe || exited >= 0:
		return rebuilt
	default:
		return notReady
	}
}

// renameSafe reports whether go renames a linked binary into stageDir
// rather than writing it in place: not on Windows, not into a setgid dir,
// and not from a work dir under GOCACHE, which cmd/go copies from.
func renameSafe(goos string, stageDir os.FileInfo, goTmp, goCache string) bool {
	if goos == "windows" || goCache == "" || strings.HasPrefix(goTmp, goCache) {
		return false
	}
	return stageDir == nil || stageDir.Mode()&fs.ModeSetgid == 0
}

// publish makes a rebuilt binary the cache entry, by a rename: a run
// executing the old entry keeps it. A confirmed copy marks its entry used,
// since go's touch reached only the copy. The cache is an optimization:
// whatever fails here is dropped.
func publish(b *stagedBinary, r readiness) {
	if b.Cached == "" {
		return
	}
	switch {
	case r == rebuilt:
		unlock, err := lockFile(b.Cached + ".lock")
		if err != nil {
			return
		}
		defer unlock()
		sum, err := sumFile(b.Path)
		if err != nil {
			return
		}
		tmp := b.Cached + ".tmp-" + strconv.Itoa(os.Getpid())
		if _, err := linkOrCopy(b.Path, tmp); err != nil {
			_ = os.Remove(tmp)
			return
		}
		if err := os.Rename(tmp, b.Cached); err != nil {
			_ = os.Remove(tmp)
			return
		}
		_ = writeSum(b.Cached, sum)
		if b.damaged {
			fmt.Fprintf(os.Stderr, "gotest: the cached test binary of %s did not match its record and was linked again\n", b.Package)
		}
	case r == confirmed && b.placed != nil && !b.linked:
		now := time.Now()
		_ = os.Chtimes(b.Cached, now, now)
	}
}
