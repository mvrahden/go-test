package gotestrunner_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"time"

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
type BuildFailureVerdictsTestSuite struct{}

type buildFailureCtx struct{ tmpDir string }

func (s *BuildFailureVerdictsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *BuildFailureVerdictsTestSuite) BeforeEach(t *gotest.T) *buildFailureCtx {
	dir, err := os.MkdirTemp("", "gotest-buildfailure-*")
	gotest.NoError(t, err)
	return &buildFailureCtx{tmpDir: dir}
}

func (s *BuildFailureVerdictsTestSuite) AfterEach(_ *gotest.T, c *buildFailureCtx) {
	os.RemoveAll(c.tmpDir)
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

func (s *BuildFailureVerdictsTestSuite) TestBrokenPackageMessage(t *gotest.T, c *buildFailureCtx) {
	t.It("renders diagnostics in the go build shape", func(it *gotest.T) {
		msg := gotestrunner.ExportBrokenPackageMessage(&gotestgen.BrokenPackage{
			PkgPath: "example.com/broken",
			Errors:  []string{"a.go:1:1: first", "b.go:2:2: second"},
		})
		gotest.Equal(it, "# example.com/broken\na.go:1:1: first\nb.go:2:2: second\n", msg)
	})
}

func (s *BuildFailureVerdictsTestSuite) TestCollectorBooksBrokenPackages(t *gotest.T, c *buildFailureCtx) {
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

func (s *BuildFailureVerdictsTestSuite) TestPipelineFailsOnBrokenPackages(t *gotest.T, c *buildFailureCtx) {
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
		}, brokenOverlay(c.tmpDir))
		gotest.NoError(sub, err)
		gotest.Equal(sub, 2, result.ExitCode,
			"a run with an unbuildable package must exit 2, never report success")

		events, perr := gotestspec.ParseEvents(bytes.NewReader(result.CapturedJSON))
		gotest.NoError(sub, perr)
		gotest.True(sub, gotestspec.HasFailures(gotestspec.BuildTree(events)))
	}
}

func (s *BuildFailureVerdictsTestSuite) TestPipelineCleanWhenNothingMatched(t *gotest.T, c *buildFailureCtx) {
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
		}, &gotestrunner.OverlayResult{WorkDir: c.tmpDir})
		gotest.NoError(sub, err)
		gotest.Equal(sub, 0, result.ExitCode,
			"no matched packages and no failures is the only clean empty run")
	}
}

func (s *BuildFailureVerdictsTestSuite) TestPipelineCutShortBeforeAnySuite(t *gotest.T, c *buildFailureCtx) {
	for sub, tC := range gotest.Each(t, []struct {
		Desc      string
		streaming bool
		deadline  bool
		want      int
	}{
		{"batch mode, interrupted", false, false, 130},
		{"batch mode, past its deadline", false, true, 1},
		{"streaming mode, interrupted", true, false, 130},
		{"streaming mode, past its deadline", true, true, 1},
	}) {
		ctx, cancel := context.WithCancel(context.Background())
		if tC.deadline {
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}
		cancel()
		result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
			Streaming:     tC.streaming,
			OutputMode:    gotestrunner.RunCaptureJSON,
			GlobalTimeout: time.Minute,
		}, &gotestrunner.OverlayResult{WorkDir: c.tmpDir})
		gotest.NoError(sub, err)
		gotest.Equal(sub, tC.want, result.ExitCode, "a run that never reached a verdict must not exit 0")
	}
}

func (s *BuildFailureVerdictsTestSuite) TestExitCodeAfterDispatch(t *gotest.T, _ *buildFailureCtx) {
	for sub, tC := range gotest.Each(t, []struct {
		Desc  string
		worst int
		err   error
		want  int
	}{
		{"no cancellation keeps the verdict", 1, nil, 1},
		{"an interrupt is 130 over a green run", 0, context.Canceled, 130},
		{"an interrupt is 130 over the failures it caused", 1, context.Canceled, 130},
		{"an interrupt is 130 over a build failure", 2, context.Canceled, 130},
		{"a deadline keeps the verdict for the deadline failure", 0, context.DeadlineExceeded, 0},
		{"a deadline keeps a red verdict", 1, context.DeadlineExceeded, 1},
	}) {
		gotest.Equal(sub, tC.want, gotestrunner.ExportExitCodeAfterDispatch(tC.worst, tC.err))
	}
}

func (s *BuildFailureVerdictsTestSuite) TestDeadlineFailsTheRun(t *gotest.T, _ *buildFailureCtx) {
	t.When("the global --timeout expires before a green run's last verdict", func(w *gotest.T) {
		result := gotestrunner.PipelineResult{CapturedJSON: []byte{}}
		gotestrunner.ExportApplyDeadlineFailure(&result, 3*time.Second, context.DeadlineExceeded, nil)

		w.It("exits 1 and books the timeout into the stream every renderer reads", func(it *gotest.T) {
			gotest.Equal(it, 1, result.ExitCode)
			gotest.Contains(it, string(result.CapturedJSON), `{"Action":"output","Package":"global --timeout","Output":"FAIL: global --timeout exceeded after 3s\n"}`)
			gotest.Contains(it, string(result.CapturedJSON), `{"Action":"fail","Package":"global --timeout"}`)
		})
	})

	t.When("the run was red already", func(w *gotest.T) {
		result := gotestrunner.PipelineResult{ExitCode: 2, CapturedJSON: []byte{}}
		gotestrunner.ExportApplyDeadlineFailure(&result, 3*time.Second, context.DeadlineExceeded, nil)

		w.It("keeps its exit code", func(it *gotest.T) {
			gotest.Equal(it, 2, result.ExitCode)
		})
	})

	t.When("it names the units it cut short", func(w *gotest.T) {
		result := gotestrunner.PipelineResult{CapturedJSON: []byte{}}
		gotestrunner.ExportApplyDeadlineFailure(&result, 3*time.Second, context.DeadlineExceeded, []gotestrunner.CensusCase{{Pkg: "example.com/pkg", Path: "TestXTestSuite/TestHang"}})

		w.It("exits 1 and books no synthetic package: the units carry the failure", func(it *gotest.T) {
			gotest.Equal(it, 1, result.ExitCode)
			gotest.Empty(it, result.CapturedJSON)
		})
	})

	t.When("no deadline cut the run short", func(w *gotest.T) {
		for sub, err := range gotest.Each(w, []error{nil, context.Canceled}) {
			result := gotestrunner.PipelineResult{CapturedJSON: []byte{}}
			gotestrunner.ExportApplyDeadlineFailure(&result, 3*time.Second, err, nil)
			gotest.Equal(sub, 0, result.ExitCode)
			gotest.Empty(sub, result.CapturedJSON)
		}
	})
}
