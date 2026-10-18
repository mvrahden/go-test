package gotest_test

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DiffMemoryTestSuite: a mismatch between long values is diffed in space
// linear in their size, not in a table of their product.
// Sequential: Setenv, and one redirected snapshot dir.
type DiffMemoryTestSuite struct {
	restore func()
}

func (s *DiffMemoryTestSuite) BeforeEach(t *gotest.T) {
	s.restore = gotest.ExportRedirectSnapshots(t.TempDir())
}

func (s *DiffMemoryTestSuite) AfterEach(_ *gotest.T) { s.restore() }

// text is a value whose %#v rendering spans its lines, the shape Equal diffs.
type text []string

func (v text) GoString() string { return strings.Join(v, "\n") }

func tenThousandLines() (expected, actual text) {
	expected = make(text, 10000)
	for i := range expected {
		expected[i] = "line " + strconv.Itoa(i)
	}
	actual = append(text(nil), expected...)
	actual[5000] = "line five thousand, changed"
	actual[9000] += " and more"
	return expected, actual
}

// allocatedBy measures what fn allocates, in bytes.
func allocatedBy(fn func()) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func (s *DiffMemoryTestSuite) TestLargeMismatch(t *gotest.T) {
	t.Setenv(protocol.EnvCI, "0")
	expected, actual := tenThousandLines()
	const limit = 64 << 20

	t.When("Equal compares two 10k-line values that differ in two places", func(w *gotest.T) {
		var m *gotest.R
		allocated := allocatedBy(func() { m = gotest.Record(func(r *gotest.R) { gotest.Equal(r, expected, actual) }) })
		w.It("fails with a diff naming both changes and allocates under 64 MB doing so", func(it *gotest.T) {
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "+line five thousand, changed")
			gotest.Contains(it, m.Message(), "+line 9000 and more")
			gotest.Less(it, allocated, uint64(limit), "the diff allocated %d MB", allocated>>20)
		})
	})

	t.When("MatchSnapshot compares 10k lines against a baseline that differs in two places", func(w *gotest.T) {
		gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, expected.GoString(), "big") })
		var m *gotest.R
		allocated := allocatedBy(func() { m = gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, actual.GoString(), "big") }) })
		w.It("fails with a diff naming both changes and allocates under 64 MB doing so", func(it *gotest.T) {
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "+line five thousand, changed")
			gotest.Less(it, allocated, uint64(limit), "the diff allocated %d MB", allocated>>20)
		})
	})
}
