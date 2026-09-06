package gotestspec_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"

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

func (s *CoverageTestSuite) TestParseCoverageReader(t *gotest.T) {
	profile := `mode: atomic
github.com/user/repo/pkg/foo/foo.go:10.20,12.2 1 5
github.com/user/repo/pkg/foo/foo.go:14.30,16.2 1 0
github.com/user/repo/pkg/bar/bar.go:5.10,8.2 3 1
github.com/user/repo/pkg/bar/bar.go:10.10,12.2 1 0
`
	report, err := gotestspec.ExportParseCoverageReader(strings.NewReader(profile))
	gotest.NoError(t, err)
	gotest.Len(t, report.Packages, 2)

	// Sorted by path.
	bar, foo := report.Packages[0], report.Packages[1]
	gotest.Equal(t, "github.com/user/repo/pkg/bar", bar.Path)
	gotest.Equal(t, 3, bar.Covered)
	gotest.Equal(t, 4, bar.Total)
	gotest.InDelta(t, 75.0, bar.Percentage, 0.1)
	gotest.Equal(t, "github.com/user/repo/pkg/foo", foo.Path)
	gotest.Equal(t, 1, foo.Covered)
	gotest.Equal(t, 2, foo.Total)
	gotest.InDelta(t, 50.0, foo.Percentage, 0.1)
	// Total: 4 covered of 6.
	gotest.InDelta(t, 66.7, report.Total, 0.1)
}

func (s *CoverageTestSuite) TestParseCoverageReader_Empty(t *gotest.T) {
	report, err := gotestspec.ExportParseCoverageReader(strings.NewReader("mode: set\n"))
	gotest.NoError(t, err)
	gotest.Zero(t, report.Total)
	gotest.Empty(t, report.Packages)
}

func (s *CoverageTestSuite) TestParseCoverageLine(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc  string
		input string
		file  string
		block string
		stmts int
		count int
		err   bool
	}{
		{Desc: "standard line", input: "github.com/user/repo/foo.go:10.20,12.2 1 5", file: "github.com/user/repo/foo.go", block: "10.20,12.2", stmts: 1, count: 5},
		{Desc: "uncovered", input: "pkg/bar.go:5.1,8.3 3 0", file: "pkg/bar.go", block: "5.1,8.3", stmts: 3, count: 0},
		{Desc: "empty line", input: "", err: true},
	}) {
		file, block, stmts, count, err := gotestspec.ExportParseCoverageLine(tc.input)
		if tc.err {
			gotest.Error(sub, err)
			continue
		}
		gotest.NoError(sub, err)
		gotest.Equal(sub, tc.file, file)
		gotest.Equal(sub, tc.block, block)
		gotest.Equal(sub, tc.stmts, stmts)
		gotest.Equal(sub, tc.count, count)
	}
}

func (s *CoverageTestSuite) TestRenderSummary_WithCoverage(t *gotest.T) {
	packages := []*gotestspec.Package{{
		Path:     "p",
		Duration: time.Second,
		Nodes:    []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "A", Status: gotestspec.StatusPass}},
	}}
	report := &gotestspec.CoverageReport{
		Total:    82.4,
		Packages: []gotestspec.PackageCoverage{{Path: "p", Percentage: 82.4, Covered: 41, Total: 50}},
	}

	var buf bytes.Buffer
	gotestspec.RenderSummary(&buf, packages, gotestspec.WithNoColor(), gotestspec.WithCoverage(report))

	gotest.Contains(t, buf.String(), "Coverage: 82.4%")
}

func (s *CoverageTestSuite) TestRenderMarkdownSummary_WithCoverage(t *gotest.T) {
	packages := []*gotestspec.Package{{
		Path:     "p",
		Duration: time.Second,
		Nodes:    []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "A", Status: gotestspec.StatusPass}},
	}}
	report := &gotestspec.CoverageReport{
		Total: 75.0,
		Packages: []gotestspec.PackageCoverage{
			{Path: "pkg/foo", Percentage: 90.0, Covered: 9, Total: 10},
			{Path: "pkg/bar", Percentage: 60.0, Covered: 6, Total: 10},
		},
	}

	var buf bytes.Buffer
	gotestspec.RenderMarkdownSummary(&buf, packages, gotestspec.WithCoverage(report))
	out := buf.String()

	gotest.Contains(t, out, "### Coverage: 75.0%")
	gotest.Contains(t, out, "| `pkg/foo` | 90.0% |")
	gotest.Contains(t, out, "| `pkg/bar` | 60.0% |")
}
