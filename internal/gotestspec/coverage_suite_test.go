package gotestspec_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CoverageTestSuite tests coverage profile parsing, in particular the
// deduplication of blocks that appear multiple times in merged profiles.
type CoverageTestSuite struct{}

func (s *CoverageTestSuite) TestProfileBlockDeduplication(t *gotest.T) {
	writeProfile := func(it *gotest.T, content string) string {
		path := filepath.Join(it.TempDir(), "cover.out")
		err := os.WriteFile(path, []byte(content), 0o600)
		gotest.NoError(it, err)
		return path
	}

	t.When("a merged profile repeats the same block", func(w *gotest.T) {
		w.It("counts the block once with the maximum execution count", func(it *gotest.T) {
			// foo/bar.go:10.2,12.3 appears twice (counts 0 and 3): it must be
			// counted once, as covered (max count wins, matching go tool cover).
			path := writeProfile(it, "mode: set\n"+
				"foo/bar.go:10.2,12.3 2 0\n"+
				"foo/bar.go:20.2,22.3 3 1\n"+
				"foo/bar.go:10.2,12.3 2 3\n"+
				"foo/baz.go:5.1,6.2 1 0\n")

			report, err := gotestspec.ParseCoverageProfile(path)
			gotest.NoError(it, err)

			gotest.Len(it, report.Packages, 1)
			pkg := report.Packages[0]
			gotest.Equal(it, "foo", pkg.Path)
			gotest.Equal(it, 5, pkg.Covered)
			gotest.Equal(it, 6, pkg.Total)
			gotest.InDelta(it, 83.3, pkg.Percentage, 0.1)
			gotest.InDelta(it, 83.3, report.Total, 0.1)
		})

		w.It("keeps a block uncovered when all duplicates have count zero", func(it *gotest.T) {
			path := writeProfile(it, "mode: set\n"+
				"foo/bar.go:10.2,12.3 2 0\n"+
				"foo/bar.go:10.2,12.3 2 0\n"+
				"foo/bar.go:20.2,22.3 1 1\n")

			report, err := gotestspec.ParseCoverageProfile(path)
			gotest.NoError(it, err)

			gotest.Len(it, report.Packages, 1)
			pkg := report.Packages[0]
			gotest.Equal(it, 1, pkg.Covered)
			gotest.Equal(it, 3, pkg.Total)
		})
	})

	t.When("a profile contains no duplicate blocks", func(w *gotest.T) {
		w.It("reports the same totals as before", func(it *gotest.T) {
			path := writeProfile(it, "mode: atomic\n"+
				"foo/bar.go:10.2,12.3 2 5\n"+
				"foo/baz.go:5.1,6.2 1 0\n")

			report, err := gotestspec.ParseCoverageProfile(path)
			gotest.NoError(it, err)

			gotest.Len(it, report.Packages, 1)
			pkg := report.Packages[0]
			gotest.Equal(it, 2, pkg.Covered)
			gotest.Equal(it, 3, pkg.Total)
		})
	})
}
