package main_test

import (
	"os"
	"path/filepath"
	"strings"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SummaryBadgeTestSuite covers `summary --badge=<file>`: the flag is
// accepted, forces a coverage profile when none is requested, and renders
// the badge from whatever profile the summary reports on.
type SummaryBadgeTestSuite struct{}

func (s *SummaryBadgeTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type badgeCtx struct{ dir string }

func (s *SummaryBadgeTestSuite) BeforeEach(t *gotest.T) *badgeCtx {
	return &badgeCtx{dir: t.TempDir()}
}

func (s *SummaryBadgeTestSuite) TestFlagIsASummaryValueFlag(t *gotest.T, _ *badgeCtx) {
	t.It("is registered as a value flag", func(it *gotest.T) {
		gotest.Equal(it, main.ValueFlag, main.ExportGotestFlags["--badge"])
	})

	t.It("is accepted by summary and routed to gotest's own args", func(it *gotest.T) {
		own, goTest, err := main.SplitArgs([]string{"--badge=coverage.svg", "-race", "./..."}, main.ExportSummaryAllowed)
		gotest.NoError(it, err)
		gotest.Equal(it, []string{"--badge=coverage.svg"}, own)
		gotest.Equal(it, []string{"-race", "./..."}, goTest)
	})

	t.It("is rejected by the plain run mode", func(it *gotest.T) {
		_, _, err := main.SplitArgs([]string{"--badge=coverage.svg"}, main.ExportTestAllowed)
		gotest.Error(it, err)
	})
}

func (s *SummaryBadgeTestSuite) TestBadgeForcesACoverageProfile(t *gotest.T, _ *badgeCtx) {
	t.When("neither --min nor -coverprofile asks for coverage", func(w *gotest.T) {
		w.It("leaves the go test args alone when no badge is requested", func(it *gotest.T) {
			args, profile, cleanup, err := main.ExportEnsureCoverProfile([]string{"-race"}, false)
			cleanup()
			gotest.NoError(it, err)
			gotest.Empty(it, profile)
			gotest.Equal(it, []string{"-race"}, args)
		})

		w.It("adds a temporary -coverprofile when one is needed", func(it *gotest.T) {
			args, profile, cleanup, err := main.ExportEnsureCoverProfile([]string{"-race"}, true)
			cleanup()
			gotest.NoError(it, err)
			gotest.NotEmpty(it, profile)
			gotest.Equal(it, []string{"-race", "-coverprofile=" + profile}, args)
		})
	})

	t.When("the caller already passes -coverprofile", func(w *gotest.T) {
		w.It("reuses that profile", func(it *gotest.T) {
			args, profile, cleanup, err := main.ExportEnsureCoverProfile([]string{"-coverprofile=c.out"}, true)
			cleanup()
			gotest.NoError(it, err)
			gotest.Equal(it, "c.out", profile)
			gotest.Equal(it, []string{"-coverprofile=c.out"}, args)
		})
	})
}

func (s *SummaryBadgeTestSuite) TestRendersBadgeFromReportedProfile(t *gotest.T, ctx *badgeCtx) {
	stream := filepath.Join(ctx.dir, "events.json")
	gotest.NoError(t, os.WriteFile(stream, []byte(`{"Action":"run","Package":"example.com/pkg","Test":"TestOK"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestOK"}
{"Action":"pass","Package":"example.com/pkg"}
`), 0o600))
	profile := filepath.Join(ctx.dir, "cover.out")
	// 3 of 4 statements covered: 75.0%.
	gotest.NoError(t, os.WriteFile(profile, []byte("mode: set\n"+
		"example.com/pkg/a.go:1.1,2.2 3 1\n"+
		"example.com/pkg/a.go:3.1,4.2 1 0\n"), 0o600))
	out := filepath.Join(ctx.dir, "summary.txt")
	badge := filepath.Join(ctx.dir, "nested", "coverage.svg")

	t.When("summary replays a stream with a coverage profile", func(w *gotest.T) {
		code := main.ExportRunSummaryFromInput(stream, "terminal", out, profile, true, false, true, badge)

		w.It("exits by the stream's verdict", func(it *gotest.T) {
			gotest.Equal(it, 0, code)
		})

		w.It("writes the badge, creating parent directories, with the profile's total", func(it *gotest.T) {
			data, err := os.ReadFile(badge)
			gotest.NoError(it, err)
			svg := string(data)
			gotest.True(it, strings.HasPrefix(svg, "<svg"), "not an svg: %q", svg)
			gotest.Contains(it, svg, ">75.0%<")
		})
	})

	t.When("summary has no coverage profile to report on", func(w *gotest.T) {
		noBadge := filepath.Join(ctx.dir, "none.svg")
		main.ExportRunSummaryFromInput(stream, "terminal", out, "", true, false, true, noBadge)

		w.It("writes no badge rather than a badge for 0%", func(it *gotest.T) {
			_, err := os.Stat(noBadge)
			gotest.ErrorIs(it, err, os.ErrNotExist)
		})
	})
}
