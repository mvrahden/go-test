// Ring 0: raw checks only, so a broken assertion engine cannot pass its own
// tests. No gotest.* assertion may appear in this file.
package gotest_test //nolint:fail-guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SnapshotInternalsTestSuite covers the machinery below MatchSnapshot:
// caller-package detection and its cache, test-name splitting, the read-only
// CI switch and CRLF normalization.
// Sequential: Setenv.
type SnapshotInternalsTestSuite struct {
	dir     string
	snapDir string
	restore func()
}

// BeforeEach redirects snapshots into a temp dir, so no test writes into
// the source tree.
func (s *SnapshotInternalsTestSuite) BeforeEach(t *gotest.T) {
	// The package directory, where go test runs the binary: the caller's file
	// would do as well, until -trimpath makes it a path nothing can open.
	dir, err := os.Getwd()
	gotest.NoError(t, err)
	s.dir = dir
	s.snapDir = t.TempDir()
	s.restore = gotest.ExportRedirectSnapshots(s.snapDir)
}

func (s *SnapshotInternalsTestSuite) AfterEach(t *gotest.T) { s.restore() }

// snap returns the path of a snapshot file in the test's snapshot dir.
func (s *SnapshotInternalsTestSuite) snap(name string) string {
	return filepath.Join(s.snapDir, name)
}

// Without a redirect, snapshots live in testdata/__snapshots__ beside the
// file MatchSnapshot is called from.
func (s *SnapshotInternalsTestSuite) TestDefaultSnapshotDir(t *gotest.T) {
	caller := filepath.Join(s.dir, "x_test.go")
	want := filepath.Join(s.dir, "testdata", "__snapshots__")
	if got := gotest.ExportDefaultSnapshotDir(caller); got != want {
		t.Errorf("snapshot dir: want %s, got %s", want, got)
	}
}

func fatalf(t *gotest.T, format string, args ...any) {
	t.Errorf(format, args...)
	t.FailNow()
}

func (s *SnapshotInternalsTestSuite) TestIsExternalPackage(t *gotest.T) {
	ptest := filepath.Join(s.dir, "export_test.go")
	pxtest := filepath.Join(s.dir, "snapshot_internals_suite_test.go")

	t.It("reports false for a ptest file", func(it *gotest.T) {
		gotest.ExportPkgCache.Delete(ptest)
		if gotest.ExportIsExternalPackage(ptest) {
			it.Errorf("expected false for ptest file %s", ptest)
		}
	})

	t.It("reports true for a pxtest file", func(it *gotest.T) {
		gotest.ExportPkgCache.Delete(pxtest)
		if !gotest.ExportIsExternalPackage(pxtest) {
			it.Errorf("expected true for pxtest file %s", pxtest)
		}
	})

	t.It("reports false for a nonexistent file", func(it *gotest.T) {
		if gotest.ExportIsExternalPackage(filepath.Join(s.dir, "nonexistent.go")) {
			it.Errorf("expected false for nonexistent file")
		}
	})

	t.It("caches the result", func(it *gotest.T) {
		gotest.ExportPkgCache.Delete(ptest)
		gotest.ExportIsExternalPackage(ptest)
		if _, ok := gotest.ExportPkgCache.Load(ptest); !ok {
			it.Errorf("expected result to be cached")
		}
	})
}

func (s *SnapshotInternalsTestSuite) TestSplitTestName(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc         string
		input        string
		wantTopLevel string
		wantRest     string
	}{
		{Desc: "top-level only", input: "TestFoo", wantTopLevel: "TestFoo", wantRest: ""},
		{Desc: "with subtest", input: "TestFoo/bar", wantTopLevel: "TestFoo", wantRest: "bar"},
		{Desc: "nested subtests", input: "TestFoo/bar/baz", wantTopLevel: "TestFoo", wantRest: "bar/baz"},
		{Desc: "keeps a twin's number", input: "TestFoo/bar#01", wantTopLevel: "TestFoo", wantRest: "bar#01"},
		{Desc: "keeps a nested twin's number", input: "TestFoo/bar/baz#03", wantTopLevel: "TestFoo", wantRest: "bar/baz#03"},
		{Desc: "keeps an unnamed row's number", input: "TestFoo/#0", wantTopLevel: "TestFoo", wantRest: "#0"},
		{Desc: "keeps any other hash", input: "TestFoo/bar#notnum", wantTopLevel: "TestFoo", wantRest: "bar#notnum"},
	}) {
		topLevel, rest := gotest.ExportSplitTestName(tc.input)
		if topLevel != tc.wantTopLevel {
			sub.Errorf("topLevel: want %q, got %q", tc.wantTopLevel, topLevel)
		}
		if rest != tc.wantRest {
			sub.Errorf("rest: want %q, got %q", tc.wantRest, rest)
		}
	}
}

// The suffix follows the file MatchSnapshot is called from: a ptest caller
// writes TestX.snap, a pxtest caller writes TestX_ext.snap.
func (s *SnapshotInternalsTestSuite) TestMatchSnapshot_PtestUsesNoSuffix(t *gotest.T) {
	// The CLI arms CI mode from CI=true and read-only snapshots would refuse
	// the fresh baseline this test writes; GOTEST_CI=0 is the documented opt-out.
	t.Setenv("GOTEST_CI", "0")
	plain := s.snap("TestSnapshotInternalsTestSuite.snap")
	ext := s.snap("TestSnapshotInternalsTestSuite_ext.snap")

	gotest.ExportMatchSnapshotFromPtest(t, "ptest-value")

	data, err := os.ReadFile(plain)
	if err != nil {
		fatalf(t, "expected .snap (no _ext suffix): %v", err)
	}
	if !strings.Contains(string(data), "ptest-value") {
		fatalf(t, "expected snapshot content, got:\n%s", data)
	}
	if _, err := os.Stat(ext); err == nil {
		fatalf(t, "_ext.snap should not exist for a ptest caller")
	}

	gotest.MatchSnapshot(t, "pxtest-value")

	if _, err := os.Stat(ext); err != nil {
		fatalf(t, "expected _ext.snap for a pxtest caller: %v", err)
	}
}

func (s *SnapshotInternalsTestSuite) TestMatchSnapshot_NormalizesCRLFInContent(t *gotest.T) {
	// The CLI arms CI mode from CI=true and read-only snapshots would refuse
	// the fresh baseline this test writes; GOTEST_CI=0 is the documented opt-out.
	t.Setenv("GOTEST_CI", "0")
	snapFile := s.snap("TestSnapshotInternalsTestSuite_ext.snap")

	gotest.MatchSnapshot(t, "line1\r\nline2\r\n")

	data, err := os.ReadFile(snapFile)
	if err != nil {
		fatalf(t, "expected snapshot file: %v", err)
	}
	if strings.Contains(string(data), "\r\n") {
		fatalf(t, "snapshot file must not contain \\r\\n: content is normalized on write")
	}
	if !strings.Contains(string(data), "line1\nline2\n") {
		fatalf(t, "expected normalized content in snapshot file, got:\n%s", data)
	}

	gotest.MatchSnapshot(t, "line1\r\nline2\r\n")
}

func (s *SnapshotInternalsTestSuite) TestMatchSnapshot_MatchesAfterCRLFFileCorruption(t *gotest.T) {
	// The CLI arms CI mode from CI=true and read-only snapshots would refuse
	// the fresh baseline this test writes; GOTEST_CI=0 is the documented opt-out.
	t.Setenv("GOTEST_CI", "0")
	snapFile := s.snap("TestSnapshotInternalsTestSuite_ext.snap")

	gotest.MatchSnapshot(t, "stable value")

	data, err := os.ReadFile(snapFile)
	if err != nil {
		fatalf(t, "read snap: %v", err)
	}
	corrupted := strings.ReplaceAll(string(data), "\n", "\r\n")
	if err := os.WriteFile(snapFile, []byte(corrupted), 0o600); err != nil {
		fatalf(t, "write corrupted snap: %v", err)
	}

	gotest.MatchSnapshot(t, "stable value")
}

func (s *SnapshotInternalsTestSuite) TestSnapshotReadonly(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		value    string
		readonly bool
	}{
		{Desc: "GOTEST_CI=1 is readonly", value: "1", readonly: true},
		{Desc: "GOTEST_CI=true is readonly", value: "true", readonly: true},
		{Desc: "GOTEST_CI=0 is writable", value: "0", readonly: false},
		{Desc: "GOTEST_CI unset is writable", value: "", readonly: false},
	}) {
		sub.Setenv("GOTEST_CI", tc.value)
		if got := gotest.ExportSnapshotReadonly(); got != tc.readonly {
			sub.Errorf("GOTEST_CI=%q: readonly = %v, want %v", tc.value, got, tc.readonly)
		}
	}
}

// mockT records what MatchSnapshot reports instead of failing the test.
type mockT struct {
	name   string
	failed bool
	msg    string
}

func (m *mockT) Helper()      {}
func (m *mockT) FailNow()     {}
func (m *mockT) Name() string { return m.name }
func (m *mockT) Errorf(format string, args ...any) {
	m.failed = true
	m.msg = fmt.Sprintf(format, args...)
}

func (s *SnapshotInternalsTestSuite) TestMatchSnapshot_CIMode_FailsOnMissingBaseline(t *gotest.T) {
	snapFile := s.snap("TestMatchSnapshot_CIMode_FailsOnMissingBaseline_ext.snap")
	s.snap("TestMatchSnapshot_CIMode_FailsOnMissingBaseline.snap")
	t.Setenv("GOTEST_CI", "1")

	mock := &mockT{name: "TestMatchSnapshot_CIMode_FailsOnMissingBaseline/subtest"}
	gotest.MatchSnapshot(mock, "new-value")

	if !mock.failed {
		fatalf(t, "expected MatchSnapshot to fail in CI mode when no baseline exists")
	}
	if !strings.Contains(mock.msg, "no baseline snapshot") {
		fatalf(t, "expected 'no baseline snapshot' message, got: %s", mock.msg)
	}
	if _, err := os.Stat(snapFile); err == nil {
		fatalf(t, "expected snapshot file to NOT be written in CI mode")
	}
}

func (s *SnapshotInternalsTestSuite) TestMatchSnapshot_CIMode_ComparesExistingBaseline(t *gotest.T) {
	s.snap("TestSnapshotInternalsTestSuite_ext.snap")

	// Write the baseline in writable mode, then compare under CI mode.
	t.Setenv("GOTEST_CI", "0")
	gotest.MatchSnapshot(t, "expected value")

	t.Setenv("GOTEST_CI", "1")
	gotest.MatchSnapshot(t, "expected value")
}

// The _ext suffix follows the caller's package as the function symbol names
// it, which -trimpath leaves intact where it rewrites the file's path.
func (s *SnapshotInternalsTestSuite) TestExternalBySymbol(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc   string
		symbol string
		ext    bool
		known  bool
	}{
		{Desc: "an external test package's method", symbol: "github.com/x/pkg_test.(*Suite).TestOne", ext: true, known: true},
		{Desc: "a closure inside it", symbol: "github.com/x/pkg_test.(*Suite).TestOne.func1", ext: true, known: true},
		{Desc: "the package's own test file", symbol: "github.com/x/pkg.(*Suite).TestOne", ext: false, known: true},
		{Desc: "a package whose name ends in _test only inside its path", symbol: "github.com/x/a_test/pkg.Run", ext: false, known: true},
		{Desc: "no symbol", symbol: "", ext: false, known: false},
	}) {
		ext, known := gotest.ExportExternalBySymbol(tc.symbol)
		if ext != tc.ext || known != tc.known {
			sub.Errorf("externalBySymbol(%q): got (%v, %v), want (%v, %v)", tc.symbol, ext, known, tc.ext, tc.known)
		}
	}
}
