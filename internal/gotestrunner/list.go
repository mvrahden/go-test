package gotestrunner

import (
	"context"
	"slices"
	"strings"
)

// runList answers -list: each package's binary runs once with no test
// selected, so it prints the names its pattern matches and nothing more. No
// shared fixture starts, no suite runs, and there is no census or coverage.
func runList(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult, pf ParsedFlags) (PipelineResult, error) { //nolint:gocritic // hugeParam: stable API
	collector := NewOutputCollector(cfg.OutputMode, pf.Verbose, WithEvents(cfg.Events))
	plan := overlay.plan()
	collector.StdlibTestsByPkg = plan.StdlibTests()
	collector.list = true
	noSuite := slices.Concat(overlay.NoSuitePackages, cfg.NoTestFiles)
	order := flushOrder(cfg, overlay, noSuite)
	collector.SetFlushOrder(order)
	compiled, failures := CompilePackages(ctx, overlay.SuitePackages, overlay.OverlayFlag, pf.BuildFlags, overlay.WorkDir, binaryCacheDir(overlay.BinaryCacheRoot, pf.BuildFlags), cfg.CompileParallel)
	bookBuildFailures(collector, overlay.BrokenPackages, failures)
	bookNoSuites(collector, overlay, noSuite, order)

	slices.SortFunc(compiled, func(a, b CompileResult) int { return strings.Compare(a.Package, b.Package) })
	env := buildBaseEnv(cfg)
	runFlags := TranslateToTestBinaryFlags(pf.RunFlags)
	for i := range compiled {
		pkg := compiled[i].Package
		collector.Register(pkg, 1)
		var dir string
		if p := plan.Package(pkg); p != nil {
			dir = p.Dir
		}
		target := SuiteTarget{
			SuiteSpec:  SuiteSpec{Package: pkg, Dir: dir, RunFilter: "^$"},
			BinaryPath: compiled[i].BinaryPath,
			RunFlags:   runFlags,
			Exec:       pf.Exec,
		}
		runRecorded(ctx, collector, pkg, 0, target, env, collector.UsesTest2JSON())
	}
	collector.Finalize()
	return PipelineResult{
		ExitCode:     exitCodeAfterDispatch(collector.WorstExitCode(), ctx.Err()),
		CapturedJSON: collector.CapturedJSON(),
	}, nil
}
