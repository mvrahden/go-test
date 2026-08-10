package gotestrunner_test

import (
	"bytes"
	"context"
	"os"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BuildFailureVerdictsTestSuite covers the invariant that every matched
// package ends in exactly one verdict: a package that fails to build is a
// failed package with exit code 2, in both pipeline modes and in every
// renderer fed from the collector's stream. "no test suites to run" (exit 0)
// is reserved for runs where every matched package loaded and none had
// suites.
type BuildFailureVerdictsTestSuite struct{ tmpDir string }

func (s *BuildFailureVerdictsTestSuite) BeforeEach(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-buildfailure-*")
	gotest.NoError(t, err)
	s.tmpDir = dir
}

func (s *BuildFailureVerdictsTestSuite) AfterEach(_ *gotest.T) {
	if s.tmpDir != "" {
		os.RemoveAll(s.tmpDir)
	}
}

func brokenOverlay(workDir string) *gotestrunner.OverlayResult {
	return &gotestrunner.OverlayResult{
		WorkDir: workDir,
		BrokenPackages: []gotestgen.BrokenPackage{{
			PkgPath: "example.com/broken",
			Errors:  []string{"svc.go:4:17: cannot use 42 (untyped int constant) as string value"},
		}},
	}
}

func (s *BuildFailureVerdictsTestSuite) TestBrokenPackageMessage(t *gotest.T) {
	t.It("renders diagnostics in the go build shape", func(it *gotest.T) {
		msg := gotestrunner.ExportBrokenPackageMessage(&gotestgen.BrokenPackage{
			PkgPath: "example.com/broken",
			Errors:  []string{"a.go:1:1: first", "b.go:2:2: second"},
		})
		gotest.Equal(it, "# example.com/broken\na.go:1:1: first\nb.go:2:2: second\n", msg)
	})
}

func (s *BuildFailureVerdictsTestSuite) TestCollectorBooksBrokenPackages(t *gotest.T) {
	t.When("in batch text mode", func(w *gotest.T) {
		w.It("prints the diagnostics and a FAIL line, and exits 2", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			gotestrunner.ExportBookBuildFailures(c, brokenOverlay("").BrokenPackages, nil)

			gotest.Equal(it, 2, c.WorstExitCode())
			gotest.Contains(it, stderr.String(), "# example.com/broken")
			gotest.Contains(it, stderr.String(), "cannot use 42")
			gotest.Contains(it, stdout.String(), "FAIL\texample.com/broken")
		})
	})

	t.When("in JSON capture mode", func(w *gotest.T) {
		w.It("carries the verdict and the diagnostics in the stream", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false, gotestrunner.WithWriters(&stdout, &stderr))
			gotestrunner.ExportBookBuildFailures(c, brokenOverlay("").BrokenPackages, nil)

			gotest.Equal(it, 2, c.WorstExitCode())
			events, err := gotestspec.ParseEvents(bytes.NewReader(c.CapturedJSON()))
			gotest.NoError(it, err)
			tree := gotestspec.BuildTree(events)
			gotest.True(it, gotestspec.HasFailures(tree),
				"the failure must be derivable from the stream itself")
			gotest.Len(it, tree, 1)
			gotest.Equal(it, "example.com/broken", tree[0].Path)
			gotest.Contains(it, strings.Join(tree[0].Output, ""), "cannot use 42",
				"renderers fed the stream must carry the diagnostics")
		})
	})
}

func (s *BuildFailureVerdictsTestSuite) TestPipelineFailsOnBrokenPackages(t *gotest.T) {
	for sub, tC := range gotest.Each(t, []struct {
		Desc      string
		streaming bool
	}{
		{"batch mode", false},
		{"streaming mode", true},
	}) {
		result, err := gotestrunner.RunPipeline(context.Background(), gotestrunner.PipelineConfig{
			Streaming:  tC.streaming,
			OutputMode: gotestrunner.RunCaptureJSON,
		}, brokenOverlay(s.tmpDir))
		gotest.NoError(sub, err)
		gotest.Equal(sub, 2, result.ExitCode,
			"a run with an unbuildable package must exit 2, never report success")

		events, perr := gotestspec.ParseEvents(bytes.NewReader(result.CapturedJSON))
		gotest.NoError(sub, perr)
		gotest.True(sub, gotestspec.HasFailures(gotestspec.BuildTree(events)))
	}
}

func (s *BuildFailureVerdictsTestSuite) TestPipelineCleanWhenNothingMatched(t *gotest.T) {
	for sub, tC := range gotest.Each(t, []struct {
		Desc      string
		streaming bool
	}{
		{"batch mode", false},
		{"streaming mode", true},
	}) {
		result, err := gotestrunner.RunPipeline(context.Background(), gotestrunner.PipelineConfig{
			Streaming:  tC.streaming,
			OutputMode: gotestrunner.RunCaptureJSON,
		}, &gotestrunner.OverlayResult{WorkDir: s.tmpDir})
		gotest.NoError(sub, err)
		gotest.Equal(sub, 0, result.ExitCode,
			"no matched packages and no failures is the only clean empty run")
	}
}
