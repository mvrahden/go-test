package gotest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SnapshotKeysTestSuite: a snapshot's section is the exact subtest name, and
// the switches around it mean what they say.
// Sequential: Setenv, and one redirected snapshot dir per method.
type SnapshotKeysTestSuite struct {
	snapDir string
	restore func()
}

func (s *SnapshotKeysTestSuite) BeforeEach(t *gotest.T) {
	s.snapDir = t.TempDir()
	s.restore = gotest.ExportRedirectSnapshots(s.snapDir)
}

func (s *SnapshotKeysTestSuite) AfterEach(_ *gotest.T) { s.restore() }

func (s *SnapshotKeysTestSuite) snap(t *gotest.T) string {
	data, err := os.ReadFile(filepath.Join(s.snapDir, "TestSnapshotKeysTestSuite_ext.snap"))
	gotest.NoError(t, err)
	return string(data)
}

func (s *SnapshotKeysTestSuite) TestExactKeys(t *gotest.T) {
	t.Setenv(protocol.EnvCI, "0")
	t.When("two subtests share a name and testing numbers the second", func(w *gotest.T) {
		w.It("x", func(it *gotest.T) { gotest.MatchSnapshot(it, "first twin") })
		w.It("x", func(it *gotest.T) { gotest.MatchSnapshot(it, "second twin") })
		w.It("each keeps a section of its own", func(it *gotest.T) {
			content := s.snap(it)
			gotest.Contains(it, content, "=== SNAP TestExactKeys/two_subtests_share_a_name_and_testing_numbers_the_second/x ===\nfirst twin")
			gotest.Contains(it, content, "=== SNAP TestExactKeys/two_subtests_share_a_name_and_testing_numbers_the_second/x#01 ===\nsecond twin")
		})
	})
	t.When("Each rows have no name of their own", func(w *gotest.T) {
		for it, n := range gotest.Each(w, []int{0, 1}) {
			gotest.MatchSnapshot(it, strings.Repeat("row", n+1))
		}
		w.It("#0 and #1 keep sections of their own", func(it *gotest.T) {
			content := s.snap(it)
			gotest.Contains(it, content, "/#0 ===\nrow\n")
			gotest.Contains(it, content, "/#1 ===\nrowrow\n")
		})
	})
}

func (s *SnapshotKeysTestSuite) TestNames(t *gotest.T) {
	t.Setenv(protocol.EnvCI, "0")
	t.When("a snapshot name spans lines or spells a section header", func(w *gotest.T) {
		w.It("is refused before anything is written", func(it *gotest.T) {
			for _, bad := range []string{"two\nlines", "=== SNAP forged ==="} {
				m := gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, "v", bad) })
				gotest.True(it, m.Failed(), "name %q was accepted", bad)
				gotest.Contains(it, m.Message(), "snapshot name")
			}
			_, err := os.Stat(filepath.Join(s.snapDir, "unknown_ext.snap"))
			gotest.True(it, os.IsNotExist(err), "a refused name still wrote a file")
		})
	})
}

func (s *SnapshotKeysTestSuite) TestUpdateSwitch(t *gotest.T) {
	t.Setenv(protocol.EnvCI, "0")
	t.When("GOTEST_UPDATE_SNAPSHOTS is a false spelling of strconv.ParseBool", func(w *gotest.T) {
		w.It("a mismatch fails and the baseline is left as it was", func(it *gotest.T) {
			gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, "baseline", "k") })
			for _, off := range []string{"0", "f", "F", "false", "FALSE", "False"} {
				it.Setenv(protocol.EnvUpdateSnapshots, off)
				m := gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, "changed", "k") })
				gotest.True(it, m.Failed(), "GOTEST_UPDATE_SNAPSHOTS=%s rewrote the baseline", off)
			}
			data, err := os.ReadFile(filepath.Join(s.snapDir, "unknown_ext.snap"))
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), "baseline")
			gotest.NotContains(it, string(data), "changed")
		})
	})
	t.When("GOTEST_CI is a false spelling of strconv.ParseBool", func(w *gotest.T) {
		w.It("a missing baseline is written", func(it *gotest.T) {
			for _, off := range []string{"f", "F", "FALSE", "False"} {
				it.Setenv(protocol.EnvCI, off)
				m := gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, "v", "ci-"+off) })
				gotest.False(it, m.Failed(), "GOTEST_CI=%s refused a new baseline: %s", off, m.Message())
			}
		})
	})
}

func (s *SnapshotKeysTestSuite) TestBenchAndFuzzNames(t *gotest.T) {
	t.When("MatchSnapshot is given a benchmark's or fuzz target's wrapper", func(w *gotest.T) {
		w.It("both carry the name testing gave them", func(it *gotest.T) {
			var got, want string
			var named bool
			testing.Benchmark(func(b *testing.B) {
				n, ok := any(gotest.NewB(b)).(interface{ Name() string })
				named = ok
				if ok {
					got, want = n.Name(), b.Name()
				}
				for b.Loop() {
				}
			})
			gotest.True(it, named, "B has no Name")
			gotest.Equal(it, want, got)
			_, ok := any(gotest.NewF(nil, nil, nil, nil)).(interface{ Name() string })
			gotest.True(it, ok, "F has no Name")
		})
	})
}

// FNameTestSuite: F.Name is the name testing gave the fuzz target, the
// name a fuzz target's snapshot file is keyed by.
type FNameTestSuite struct{}

func (s *FNameTestSuite) FuzzName(f *gotest.F) {
	gotest.Regexp(f, "^FuzzFNameTestSuite.*Name$", f.F().Name())
	gotest.Equal(f, f.F().Name(), f.Name())
	f.Add(0)
	f.Fuzz(func(*gotest.T, int) {})
}
