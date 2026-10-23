package gotest

import (
	"bufio"
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
	"github.com/mvrahden/go-test/pkg/gotest/internal/snapfile"
)

var (
	pkgCache sync.Map // callerFile → bool (true if _test package)
	snapMu   sync.Map // snapPath → *sync.Mutex
)

// lineEnd is a newline and every CR before it: a single CRLF pass leaves one
// CR of a run, which the next read strips, so the baseline never matches.
var lineEnd = regexp.MustCompile("\r+\n")

// externalBySymbol reports whether the function fn names was compiled into an
// external test package: its package path ends in _test. The symbol survives
// -trimpath, which turns the caller's file into a path that cannot be opened.
func externalBySymbol(fn string) (ext, known bool) {
	if fn == "" {
		return false, false
	}
	pkg := fn[strings.LastIndex(fn, "/")+1:]
	if dot := strings.Index(pkg, "."); dot >= 0 {
		pkg = pkg[:dot]
	}
	return strings.HasSuffix(pkg, "_test"), true
}

func isExternalPackage(callerFile string) bool {
	if v, ok := pkgCache.Load(callerFile); ok {
		return v.(bool)
	}
	f, err := os.Open(callerFile)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "package ") {
			fields := strings.Fields(line)
			ext := len(fields) >= 2 && strings.HasSuffix(fields[1], "_test")
			pkgCache.Store(callerFile, ext)
			return ext
		}
	}
	if err := sc.Err(); err != nil {
		return false
	}
	pkgCache.Store(callerFile, false)
	return false
}

func fileMutex(path string) *sync.Mutex {
	mu, _ := snapMu.LoadOrStore(path, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// indentJSON pretty-prints valid JSON; anything else is returned verbatim.
func indentJSON(b []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return string(b)
	}
	return buf.String()
}

func snapshotContent(value any) (string, error) {
	if value == nil {
		return "", fmt.Errorf("unsupported snapshot value: nil")
	}
	if rv := reflect.ValueOf(value); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return "", fmt.Errorf("unsupported snapshot value: nil %T", value)
	}
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case json.RawMessage:
		// Explicit: since Go 1.27 (jsonv2) RawMessage aliases jsontext.Value,
		// which has String() and would otherwise take the Stringer branch.
		return indentJSON(v), nil
	case encoding.TextMarshaler:
		b, err := v.MarshalText()
		if err != nil {
			return "", fmt.Errorf("MarshalText failed: %w", err)
		}
		return string(b), nil
	case fmt.Stringer:
		return v.String(), nil
	case json.Marshaler:
		b, err := v.MarshalJSON()
		if err != nil {
			return "", fmt.Errorf("MarshalJSON failed: %w", err)
		}
		return indentJSON(b), nil
	case error:
		return v.Error(), nil
	case io.Reader:
		b, err := assert.ReadAndRestore(v)
		if err != nil {
			return "", fmt.Errorf("failed to read: %w", err)
		}
		return string(b), nil
	default:
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.String {
			return rv.String(), nil
		}
		return "", fmt.Errorf(
			"unsupported snapshot type %T; use string, []byte, encoding.TextMarshaler, fmt.Stringer, json.Marshaler, or error",
			value,
		)
	}
}

func matchSnapshot(t testingT, callerSkip int, value any, name ...string) {
	callerPC, callerFile, _, ok := runtime.Caller(callerSkip)
	if !ok {
		fail(t, "MatchSnapshot: unable to determine caller file", nil)
		return
	}
	callerFunc := ""
	if f := runtime.FuncForPC(callerPC); f != nil {
		callerFunc = f.Name()
	}

	content, err := snapshotContent(value)
	if err != nil {
		failf(t, "MatchSnapshot: %v", err)
		return
	}
	// CRs end lines like CRLF, the last line's too: a newline is appended below.
	content = strings.TrimSuffix(lineEnd.ReplaceAllString(content+"\n", "\n"), "\n")
	if err := snapfile.ValidateContent(content); err != nil {
		failf(t, "MatchSnapshot: %v", err)
		return
	}

	testName := "unknown"
	named, ok := t.(interface{ Name() string })
	if !ok {
		gt, ok := t.(*T)
		if ok && gt.T() != nil {
			named = gt.T()
		}
	}
	if named != nil {
		testName = named.Name()
	}
	topLevel, sub := splitTestName(testName)
	sectionKey := joinKey(sub, name)
	if strings.ContainsAny(sectionKey, "\r\n") || strings.Contains(sectionKey, "=== SNAP ") {
		failf(t, "MatchSnapshot: snapshot name %q must be one line and must not spell a section header", sectionKey)
		return
	}

	suffix := ""
	ext, known := externalBySymbol(callerFunc)
	if !known {
		ext = isExternalPackage(callerFile)
	}
	if ext {
		suffix = "_ext"
	}

	snapDir := snapshotDirFor(callerFile)
	snapPath := filepath.Join(snapDir, topLevel+suffix+".snap")

	mu := fileMutex(snapPath)
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(snapDir, 0755); err != nil {
		failf(t, "MatchSnapshot: failed to create snapshot dir: %v", err)
		return
	}

	existing, _ := os.ReadFile(snapPath)
	sections := snapfile.Parse(existing)

	claimed := claimsOf(snapPath)
	idx := sectionIndex(sections, sectionKey)
	legacy := false
	if idx < 0 {
		// A baseline the release before exact names wrote, unless another
		// key has it already.
		if old := joinKey(dedupSuffix.ReplaceAllString(sub, ""), name); old != sectionKey && !claimed[old] {
			idx = sectionIndex(sections, old)
			legacy = idx >= 0
		}
	}
	if idx >= 0 {
		claimed[sections[idx].Key] = true
	}
	claimed[sectionKey] = true

	if updateSnapshots() {
		if idx >= 0 {
			sections[idx] = snapfile.Section{Key: sectionKey, Content: content + "\n"}
		} else {
			sections = append(sections, snapfile.Section{Key: sectionKey, Content: content + "\n"})
		}
		if err := os.WriteFile(snapPath, snapfile.Serialize(sections), 0644); err != nil { //nolint:gosec // G306: not sensitive data
			failf(t, "MatchSnapshot: failed to write snapshot: %v", err)
			return
		}
		return
	}

	if idx < 0 {
		if snapshotReadonly() {
			failf(t, "MatchSnapshot: no baseline snapshot for %q — run tests locally to generate", sectionKey)
			return
		}
		sections = append(sections, snapfile.Section{Key: sectionKey, Content: content + "\n"})
		if err := os.WriteFile(snapPath, snapfile.Serialize(sections), 0644); err != nil { //nolint:gosec // G306: not sensitive data
			failf(t, "MatchSnapshot: failed to write snapshot: %v", err)
			return
		}
		return
	}

	want := sections[idx].Content
	got := content + "\n"
	if got == want && legacy && !snapshotReadonly() {
		sections[idx].Key = sectionKey
		if err := os.WriteFile(snapPath, snapfile.Serialize(sections), 0644); err != nil { //nolint:gosec // G306: not sensitive data
			failf(t, "MatchSnapshot: failed to rename snapshot: %v", err)
		}
		return
	}
	if got != want {
		if d := assert.DiffSection(want, got); d != "" {
			failf(t, "MatchSnapshot: snapshot mismatch [%s]:\n%s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", sectionKey, d)
		} else {
			failf(t, "MatchSnapshot: snapshot mismatch [%s]:\n  expected: %s\n  actual:   %s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", sectionKey, want, got)
		}
	}
}

// snapshotDirFor is where the snapshots of a caller file live. Tests of
// MatchSnapshot itself point it at a temp dir, away from the source tree.
var snapshotDirFor = defaultSnapshotDir

// defaultSnapshotDir is testdata/__snapshots__ beside the caller's file; under
// -trimpath that file is a path only the build knew, so the package directory
// go test runs the binary in stands in.
func defaultSnapshotDir(callerFile string) string {
	dir := filepath.Dir(callerFile)
	if !filepath.IsAbs(dir) || !dirExists(dir) {
		if wd, err := os.Getwd(); err == nil {
			dir = wd
		}
	}
	return filepath.Join(dir, "testdata", "__snapshots__")
}

func dirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// updateSnapshots reports whether baselines are rewritten: GOTEST_UPDATE_SNAPSHOTS
// switched on, read as protocol.SwitchOn reads it.
func updateSnapshots() bool { return protocol.SwitchOn(os.Getenv(protocol.EnvUpdateSnapshots)) }

// snapshotReadonly returns true when MatchSnapshot should refuse to generate
// new baseline snapshots. Activated by GOTEST_CI=1 (set by the --ci flag).
// Opt-out with GOTEST_CI=0 when baseline generation is intentional in CI.
func snapshotReadonly() bool { return protocol.SwitchOn(os.Getenv(protocol.EnvCI)) }

// dedupSuffix is what the release before exact names stripped from a key:
// testing's #01 on a twin and #0 on an unnamed row, or any #<digits> at the end.
var dedupSuffix = regexp.MustCompile(`#\d+$`)

// claims holds, per snapshot file, the sections a key of this process has
// read: an old-keyed section serves one key only, and never once its exact
// key has claimed it.
var claims sync.Map // snapPath → map[string]bool, guarded by fileMutex

func claimsOf(snapPath string) map[string]bool {
	m, _ := claims.LoadOrStore(snapPath, map[string]bool{})
	return m.(map[string]bool)
}

// joinKey is a section key: the subtest path and the snapshot's name, or _
// for neither.
func joinKey(sub string, name []string) string {
	key := sub
	if len(name) > 0 && name[0] != "" {
		if key != "" {
			key += " "
		}
		key += name[0]
	}
	if key == "" {
		key = "_"
	}
	return key
}

func sectionIndex(sections []snapfile.Section, key string) int {
	for i, s := range sections {
		if s.Key == key {
			return i
		}
	}
	return -1
}

// splitTestName splits a test name into its top-level function and the rest,
// kept exactly: testing's #01 on a twin and #0 on an unnamed row are part of
// the name, and each gets a section of its own.
func splitTestName(name string) (topLevel, rest string) {
	if top, sub, ok := strings.Cut(name, "/"); ok {
		return top, sub
	}
	return name, ""
}

func MatchSnapshot(t testingT, value any, name ...string) {
	matchSnapshot(t, 2, value, name...)
}
