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
	dedupSuffixRe = regexp.MustCompile(`#\d+$`)
	pkgCache      sync.Map // callerFile → bool (true if _test package)
	snapMu        sync.Map // snapPath → *sync.Mutex
)

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
		var buf bytes.Buffer
		if err := json.Indent(&buf, b, "", "  "); err == nil {
			return buf.String(), nil
		}
		return string(b), nil
	case error:
		return v.Error(), nil
	case io.Reader:
		b, err := readAndRestore(v)
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
	_, callerFile, _, ok := runtime.Caller(callerSkip)
	if !ok {
		fail(t, "MatchSnapshot: unable to determine caller file", nil)
		return
	}

	content, err := snapshotContent(value)
	if err != nil {
		failf(t, "MatchSnapshot: %v", err)
		return
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if err := snapfile.ValidateContent(content); err != nil {
		failf(t, "MatchSnapshot: %v", err)
		return
	}

	testName := "unknown"
	named, ok := t.(interface{ Name() string })
	if !ok {
		gt, ok := t.(*T)
		if ok {
			named = gt.T()
		}
	}
	if named != nil {
		testName = named.Name()
	}
	topLevel, sectionKey := splitTestName(testName)
	if len(name) > 0 && name[0] != "" {
		if sectionKey != "" {
			sectionKey += " "
		}
		sectionKey += name[0]
	}
	if sectionKey == "" {
		sectionKey = "_"
	}

	suffix := ""
	if isExternalPackage(callerFile) {
		suffix = "_ext"
	}

	snapDir := filepath.Join(filepath.Dir(callerFile), "testdata", "__snapshots__")
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

	idx := -1
	for i, s := range sections {
		if s.Key == sectionKey {
			idx = i
			break
		}
	}

	if os.Getenv(protocol.EnvUpdateSnapshots) != "" {
		if idx >= 0 {
			sections[idx].Content = content + "\n"
		} else {
			sections = append(sections, snapfile.Section{Key: sectionKey, Content: content + "\n"})
		}
		if err := os.WriteFile(snapPath, snapfile.Serialize(sections), 0644); err != nil {
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
		if err := os.WriteFile(snapPath, snapfile.Serialize(sections), 0644); err != nil {
			failf(t, "MatchSnapshot: failed to write snapshot: %v", err)
			return
		}
		return
	}

	want := sections[idx].Content
	got := content + "\n"
	if got != want {
		d := assert.Diff(want, got)
		if d != "" {
			failf(t, "MatchSnapshot: snapshot mismatch [%s]:\n  diff:\n%s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", sectionKey, d)
		} else {
			failf(t, "MatchSnapshot: snapshot mismatch [%s]:\n  expected: %s\n  actual:   %s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", sectionKey, want, got)
		}
	}
}

// snapshotReadonly returns true when MatchSnapshot should refuse to generate
// new baseline snapshots. Activated by GOTEST_CI=1 (set by the --ci flag).
// Opt-out with GOTEST_CI=0 when baseline generation is intentional in CI.
func snapshotReadonly() bool {
	v := os.Getenv(protocol.EnvCI)
	return v == "1" || v == "true"
}

func splitTestName(name string) (topLevel, rest string) {
	if top, sub, ok := strings.Cut(name, "/"); ok {
		return top, dedupSuffixRe.ReplaceAllString(sub, "")
	}
	return name, ""
}

func MatchSnapshot(t testingT, value any, name ...string) {
	matchSnapshot(t, 2, value, name...)
}
