package gotestrunner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

func computeDispatchConcurrency(runFlags *[]string, budget, totalSuites int) int {
	userParallel := ExtractParallelValue(*runFlags)

	if userParallel > 0 && budget == 0 {
		return 2 * runtime.GOMAXPROCS(0)
	}

	inter, intra := ComputeConcurrency(budget, totalSuites, runtime.GOMAXPROCS(0))
	if userParallel == 0 {
		*runFlags = InjectParallel(*runFlags, intra)
	}
	return inter
}

type PipelineConfig struct {
	GoTestArgs      []string
	SetupTimeout    time.Duration
	UpdateSnapshots bool
	CI              bool
	Parallel        int
	Streaming       bool
	OutputMode      RunMode
}

type PipelineResult struct {
	ExitCode     int
	CapturedJSON []byte
}

func RunPipeline(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult) (PipelineResult, error) {
	if !cfg.CI && os.Getenv(protocol.EnvCI) == "" && os.Getenv("CI") != "" {
		cfg.CI = true
	}
	pf := ParseExecFlags(cfg.GoTestArgs)

	if cfg.Streaming {
		return runStreaming(ctx, cfg, overlay, pf)
	}
	return runBatch(ctx, cfg, overlay, pf)
}

func buildExtraEnv(cfg PipelineConfig, proc *SharedFixtureProcess) map[string]string {
	env := make(map[string]string)
	if cfg.UpdateSnapshots {
		env[protocol.EnvUpdateSnapshots] = "1"
	}
	if cfg.CI {
		env[protocol.EnvCI] = "1"
	}
	if proc != nil {
		env[protocol.EnvSharedStateFile] = proc.StateFile()
	}
	return env
}

func buildBaseEnv(cfg PipelineConfig) []string {
	env := os.Environ()
	if cfg.UpdateSnapshots {
		env = append(env, protocol.EnvUpdateSnapshots+"=1")
	}
	if cfg.CI {
		env = append(env, protocol.EnvCI+"=1")
	}
	return env
}

func prepareTestRun(ctx context.Context, overlay *OverlayResult, buildFlags []string, setupTimeout time.Duration) ([]CompileResult, *SharedFixtureProcess, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var compiled []CompileResult
	var compileErr error
	var setupProc *SharedFixtureProcess
	var setupErr error

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		compiled, compileErr = CompilePackages(ctx, overlay.SuitePackages, overlay.OverlayFlag, buildFlags, overlay.WorkDir)
		if compileErr != nil {
			cancel()
		}
	}()

	if len(overlay.SharedFixtures) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			setupProc, setupErr = StartSharedFixtures(ctx, overlay.WorkDir, overlay.SharedFixtures, setupTimeout)
			if setupErr != nil {
				cancel()
				return
			}
			if err := setupProc.WaitAllReady(ctx, setupTimeout); err != nil {
				setupErr = err
				cancel()
			}
		}()
	}

	wg.Wait()

	if compileErr != nil || setupErr != nil {
		if setupProc != nil {
			setupProc.Teardown()
		}
		if compileErr != nil {
			return nil, nil, compileErr
		}
		return nil, nil, fmt.Errorf("shared fixture setup: %w", setupErr)
	}

	return compiled, setupProc, nil
}

func assignBudgetFiles(targets []SuiteTarget) {
	for i := range targets {
		targets[i].BudgetFile = protocol.BudgetFilePath(targets[i].BinaryPath)
	}
}

func assignCoverProfiles(targets []SuiteTarget, coverDir string) {
	for i := range targets {
		targets[i].CoverProfile = filepath.Join(coverDir, fmt.Sprintf("%d.out", i))
	}
}

func mergeCoverProfiles(targets []SuiteTarget, userProfile string) {
	var profiles []string
	for _, t := range targets {
		if t.CoverProfile != "" {
			profiles = append(profiles, t.CoverProfile)
		}
	}
	if len(profiles) == 0 {
		return
	}
	if err := MergeCoverProfiles(profiles, userProfile); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: merge cover profiles: %s\n", err)
	}
}

func setupCoverage(targets []SuiteTarget, overlay *OverlayResult, userCoverProfile string) {
	if userCoverProfile == "" {
		return
	}
	coverDir := filepath.Join(overlay.WorkDir, "cover")
	os.MkdirAll(coverDir, 0o755)
	assignCoverProfiles(targets, coverDir)
}

func runBatch(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult, pf ParsedFlags) (PipelineResult, error) {
	compiled, setupProc, err := prepareTestRun(ctx, overlay, pf.BuildFlags, cfg.SetupTimeout)
	if err != nil {
		return PipelineResult{ExitCode: 2}, err
	}
	if setupProc != nil {
		defer setupProc.Teardown()
	}

	select {
	case <-ctx.Done():
		return PipelineResult{ExitCode: 130}, nil
	default:
	}

	extraEnv := buildExtraEnv(cfg, setupProc)

	totalSuites := 0
	for _, suites := range overlay.SuitesByPkg {
		totalSuites += len(suites)
	}
	runFlags := pf.RunFlags
	if cfg.OutputMode == RunCaptureJSON {
		runFlags = append(append([]string(nil), runFlags...), "-v")
	}
	maxParallel := computeDispatchConcurrency(&runFlags, cfg.Parallel, totalSuites)

	targets := BuildSuiteTargets(compiled, overlay.SuitesByPkg, overlay.DirsByPkg, runFlags, pf.UserRunFilter)

	if len(targets) == 0 {
		if cfg.OutputMode != RunCaptureJSON {
			fmt.Fprintln(os.Stderr, "no test suites to run")
		}
		return PipelineResult{}, nil
	}

	assignBudgetFiles(targets)
	setupCoverage(targets, overlay, pf.UserCoverProfile)
	if pf.UserCoverProfile != "" {
		defer mergeCoverProfiles(targets, pf.UserCoverProfile)
	}

	collector := NewOutputCollector(cfg.OutputMode, pf.Verbose)
	collector.EmitSkippedSuites(overlay.SkippedSuitesByPkg)
	RunSuites(ctx, targets, extraEnv, maxParallel, collector)
	collector.Finalize(overlay.NoSuitePackages)

	return PipelineResult{
		ExitCode:     collector.WorstExitCode(),
		CapturedJSON: collector.CapturedJSON(),
	}, nil
}

func runStreaming(ctx context.Context, cfg PipelineConfig, overlay *OverlayResult, pf ParsedFlags) (PipelineResult, error) {
	var coverDir string
	if pf.UserCoverProfile != "" {
		coverDir = filepath.Join(overlay.WorkDir, "cover")
		os.MkdirAll(coverDir, 0o755)
	}

	baseEnv := buildBaseEnv(cfg)

	fixtureStarted := make(chan struct{})
	var setupProc *SharedFixtureProcess
	var fixtureStartErr error
	var fixtureWg sync.WaitGroup

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	if len(overlay.SharedFixtures) > 0 {
		fixtureWg.Add(1)
		go func() {
			defer fixtureWg.Done()
			var err error
			setupProc, err = StartSharedFixtures(streamCtx, overlay.WorkDir, overlay.SharedFixtures, cfg.SetupTimeout)
			if err != nil {
				fixtureStartErr = err
				streamCancel()
			}
			close(fixtureStarted)
		}()
	} else {
		close(fixtureStarted)
	}

	compileCh := CompilePackagesStream(streamCtx, overlay.SuitePackages, overlay.OverlayFlag, pf.BuildFlags, overlay.WorkDir)

	totalSuites := 0
	for _, suites := range overlay.SuitesByPkg {
		totalSuites += len(suites)
	}
	maxParallel := computeDispatchConcurrency(&pf.RunFlags, cfg.Parallel, totalSuites)
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	anyTargets := false
	var allTargets []SuiteTarget

	collector := NewOutputCollector(cfg.OutputMode, pf.Verbose)
	collector.EmitSkippedSuites(overlay.SkippedSuitesByPkg)
	collector.SetFlushOrder(overlay.SuitePackages)

loop:
	for {
		var outcome CompileOutcome
		var ok bool
		select {
		case outcome, ok = <-compileCh:
			if !ok {
				break loop
			}
		case <-streamCtx.Done():
			break loop
		}

		if outcome.Err != nil {
			continue
		}
		cr := outcome.Result

		singleCompiled := []CompileResult{cr}
		singleSuites := map[string][]string{cr.Package: overlay.SuitesByPkg[cr.Package]}
		targets := BuildSuiteTargets(singleCompiled, singleSuites, overlay.DirsByPkg, pf.RunFlags, pf.UserRunFilter)

		if len(targets) == 0 {
			continue
		}
		anyTargets = true

		assignBudgetFiles(targets)

		if pf.UserCoverProfile != "" {
			baseIdx := len(allTargets)
			for j := range targets {
				targets[j].CoverProfile = filepath.Join(coverDir, fmt.Sprintf("%d.out", baseIdx+j))
			}
			allTargets = append(allTargets, targets...)
		}

		collector.Register(cr.Package, len(targets))

		for i, target := range targets {
			wg.Add(1)
			go func(t SuiteTarget, idx int) {
				defer wg.Done()
				recorded := false
				defer func() {
					if !recorded {
						collector.RecordResult(t.Package, idx, SuiteResult{ExitCode: 1})
					}
				}()

				requiredKeys := overlay.SuiteRequiredSharedFixtureKeys[t.Package][t.SuiteName]
				var env []string
				if len(requiredKeys) > 0 {
					select {
					case <-fixtureStarted:
					case <-streamCtx.Done():
						return
					}
					if fixtureStartErr != nil {
						return
					}

					for _, key := range requiredKeys {
						ch := setupProc.Ready(key)
						if ch == nil {
							return
						}
						select {
						case <-ch:
						case <-streamCtx.Done():
							return
						}
					}

					stateFile, err := setupProc.WriteStateFileForKeys(t.SuiteName, requiredKeys)
					if err != nil {
						fmt.Fprintf(os.Stderr, "WARN: write state file for %s: %s\n", t.SuiteName, err)
						return
					}

					env = make([]string, len(baseEnv), len(baseEnv)+1)
					copy(env, baseEnv)
					env = append(env, protocol.EnvSharedStateFile+"="+stateFile)
				} else {
					env = baseEnv
				}

				select {
				case sem <- struct{}{}:
				case <-streamCtx.Done():
					return
				}
				defer func() { <-sem }()

				r := RunSingleSuite(streamCtx, t, env, collector.UsesTest2JSON())
				collector.RecordResult(t.Package, idx, r)
				recorded = true
			}(target, i)
		}
	}

	wg.Wait()
	fixtureWg.Wait()

	// Cancel streamCtx before teardown so cmd.Cancel fires on the shared
	// fixture subprocess, triggering WaitDelay-based pipe cleanup.
	streamCancel()

	if setupProc != nil {
		setupProc.Teardown()
	}

	if pf.UserCoverProfile != "" {
		mergeCoverProfiles(allTargets, pf.UserCoverProfile)
	}

	if !anyTargets && len(overlay.NoSuitePackages) == 0 {
		if cfg.OutputMode == RunBatchText {
			fmt.Fprintln(os.Stderr, "no test suites to run")
		}
		return PipelineResult{}, nil
	}

	collector.Finalize(overlay.NoSuitePackages)

	exitCode := collector.WorstExitCode()
	if ctx.Err() != nil && exitCode == 0 {
		exitCode = 130
	}

	return PipelineResult{
		ExitCode:     exitCode,
		CapturedJSON: collector.CapturedJSON(),
	}, nil
}
