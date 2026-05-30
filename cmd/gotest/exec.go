package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

type overlayResult struct {
	tmpDir                         string
	overlayFlag                    string
	sharedFixtures                 []gotestgen.SharedFixtureInfo
	suitePackages                  []string
	noSuitePackages                []string                        // loaded packages that had no suites
	suitesByPkg                    map[string][]string             // import path → suite struct names
	dirsByPkg                      map[string]string               // import path → package source directory
	suiteRequiredSharedFixtureKeys map[string]map[string][]string  // import path → test func name → required state keys
}

func generateOverlayFromLoaded(loaded []*gotestgen.LoadResult, debug bool) (*overlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateFromLoaded(loaded)
	if err != nil {
		return nil, nil, err
	}

	gotestrunner.CleanStaleOverlays()

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() { os.RemoveAll(tmpDir) }
	if debug {
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s\n", tmpDir)
		cleanup = func() {}
	}

	var suitePkgs []string
	var noSuitePkgs []string
	suitesByPkg := map[string][]string{}
	dirsByPkg := map[string]string{}
	suiteReqKeys := map[string]map[string][]string{}
	for _, r := range allResults {
		if len(r.PTest) > 0 || len(r.PXTest) > 0 {
			suitePkgs = append(suitePkgs, r.PkgPath)
		} else {
			noSuitePkgs = append(noSuitePkgs, r.PkgPath)
		}
		if len(r.SuiteNames) > 0 {
			suitesByPkg[r.PkgPath] = r.SuiteNames
		}
		if r.AbsPath != "" {
			dirsByPkg[r.PkgPath] = r.AbsPath
		}
		if len(r.SuiteRequiredSharedFixtureKeys) > 0 {
			suiteReqKeys[r.PkgPath] = r.SuiteRequiredSharedFixtureKeys
		}
	}

	return &overlayResult{
		tmpDir:                         tmpDir,
		overlayFlag:                    "-overlay=" + filepath.Join(tmpDir, "overlay.json"),
		sharedFixtures:                 allSharedFixtures,
		suitePackages:                  suitePkgs,
		noSuitePackages:                noSuitePkgs,
		suitesByPkg:                    suitesByPkg,
		dirsByPkg:                      dirsByPkg,
		suiteRequiredSharedFixtureKeys: suiteReqKeys,
	}, cleanup, nil
}

func buildExtraEnv(cfg ExecConfig, proc *SharedFixtureProcess) map[string]string {
	env := make(map[string]string)
	if cfg.UpdateSnapshots {
		env["GOTEST_UPDATE_SNAPSHOTS"] = "1"
	}
	if proc != nil {
		env["GOTEST_SHARED_STATE_FILE"] = proc.StateFile()
	}
	return env
}

func buildBaseEnv(cfg ExecConfig) []string {
	env := os.Environ()
	if cfg.UpdateSnapshots {
		env = append(env, "GOTEST_UPDATE_SNAPSHOTS=1")
	}
	return env
}

func prepareTestRun(ctx context.Context, overlay *overlayResult, buildFlags []string, setupTimeout time.Duration) ([]gotestrunner.CompileResult, *SharedFixtureProcess, error) {
	// Child context for cross-cancellation: if either goroutine fails, cancel()
	// aborts the other. Do NOT defer cancel — the shared fixture subprocess is
	// started with exec.CommandContext(ctx) and must survive until Teardown.
	ctx, cancel := context.WithCancel(ctx)

	var compiled []gotestrunner.CompileResult
	var compileErr error
	var setupProc *SharedFixtureProcess
	var setupErr error

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		compiled, compileErr = gotestrunner.CompilePackages(ctx, overlay.suitePackages, overlay.overlayFlag, buildFlags, overlay.tmpDir)
		if compileErr != nil {
			cancel()
		}
	}()

	if len(overlay.sharedFixtures) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			setupProc, setupErr = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures, setupTimeout)
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
		cancel()
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

type parsedFlags struct {
	buildFlags       []string
	runFlags         []string
	userRunFilter    string
	userCoverProfile string
	verbose          bool
}

func parseExecFlags(goTestArgs []string) parsedFlags {
	classified := gotestrunner.ClassifyGoTestArgs(goTestArgs)
	classified.BuildFlags = gotestrunner.InjectChecklinkname(classified.BuildFlags)
	verbose := gotestrunner.HasVerboseFlag(classified.RunFlags)
	userRunFilter := gotestrunner.ExtractRunFilter(classified.RunFlags)
	runFlags := gotestrunner.StripRunFilter(classified.RunFlags)
	userCoverProfile := gotestrunner.ExtractCoverProfile(runFlags)
	runFlags = gotestrunner.StripCoverProfile(runFlags)
	runFlags = gotestrunner.InjectDefaultTimeout(runFlags)
	return parsedFlags{
		buildFlags:       classified.BuildFlags,
		runFlags:         runFlags,
		userRunFilter:    userRunFilter,
		userCoverProfile: userCoverProfile,
		verbose:          verbose,
	}
}

func assignCoverProfiles(targets []gotestrunner.SuiteTarget, coverDir string) {
	for i := range targets {
		targets[i].CoverProfile = filepath.Join(coverDir, fmt.Sprintf("%d.out", i))
	}
}

func mergeCoverProfiles(targets []gotestrunner.SuiteTarget, userProfile string) {
	var profiles []string
	for _, t := range targets {
		if t.CoverProfile != "" {
			profiles = append(profiles, t.CoverProfile)
		}
	}
	if len(profiles) == 0 {
		return
	}
	if err := gotestrunner.MergeCoverProfiles(profiles, userProfile); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: merge cover profiles: %s\n", err)
	}
}

func setupCoverage(targets []gotestrunner.SuiteTarget, overlay *overlayResult, userCoverProfile string) {
	if userCoverProfile == "" {
		return
	}
	coverDir := filepath.Join(overlay.tmpDir, "cover")
	os.MkdirAll(coverDir, 0o755)
	assignCoverProfiles(targets, coverDir)
}

func executeTests(ctx context.Context, cfg ExecConfig, overlay *overlayResult) (int, error) {
	pf := parseExecFlags(cfg.GoTestArgs)

	compiled, setupProc, err := prepareTestRun(ctx, overlay, pf.buildFlags, cfg.SetupTimeout)
	if err != nil {
		return 2, err
	}
	if setupProc != nil {
		defer setupProc.Teardown()
	}

	select {
	case <-ctx.Done():
		return 130, nil
	default:
	}

	extraEnv := buildExtraEnv(cfg, setupProc)
	targets := gotestrunner.BuildSuiteTargets(compiled, overlay.suitesByPkg, overlay.dirsByPkg, pf.runFlags, pf.userRunFilter)

	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "no test suites to run")
		return 0, nil
	}

	for j := range targets {
		targets[j].BudgetFile = targets[j].BinaryPath + ".budget"
	}

	setupCoverage(targets, overlay, pf.userCoverProfile)
	if pf.userCoverProfile != "" {
		defer mergeCoverProfiles(targets, pf.userCoverProfile)
	}

	mode := gotestrunner.RunBatchText
	if cfg.JSON {
		mode = gotestrunner.RunStreamJSON
	}
	collector := gotestrunner.NewOutputCollector(mode, pf.verbose)
	gotestrunner.RunSuites(ctx, targets, extraEnv, 0, collector)
	collector.Finalize(overlay.noSuitePackages)
	return collector.WorstExitCode(), nil
}

func executeTestsStreaming(ctx context.Context, cfg ExecConfig, overlay *overlayResult) (int, error) {
	pf := parseExecFlags(cfg.GoTestArgs)

	var coverDir string
	if pf.userCoverProfile != "" {
		coverDir = filepath.Join(overlay.tmpDir, "cover")
		os.MkdirAll(coverDir, 0o755)
	}

	baseEnv := buildBaseEnv(cfg)

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	// Start fixture setup (non-blocking) and compilation concurrently.
	var setupProc *SharedFixtureProcess
	fixtureStarted := make(chan struct{})
	var fixtureStartErr error

	if len(overlay.sharedFixtures) > 0 {
		go func() {
			var err error
			setupProc, err = startSharedFixtures(streamCtx, overlay.tmpDir, overlay.sharedFixtures, cfg.SetupTimeout)
			if err != nil {
				fixtureStartErr = err
				streamCancel()
			}
			close(fixtureStarted)
		}()
	} else {
		close(fixtureStarted)
	}

	compileCh := gotestrunner.CompilePackagesStream(streamCtx, overlay.suitePackages, overlay.overlayFlag, pf.buildFlags, overlay.tmpDir)

	maxParallel := 2 * runtime.GOMAXPROCS(0)
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	anyTargets := false
	var allTargets []gotestrunner.SuiteTarget

	mode := gotestrunner.RunBatchText
	if cfg.JSON {
		mode = gotestrunner.RunStreamJSON
	}
	collector := gotestrunner.NewOutputCollector(mode, pf.verbose)
	collector.SetFlushOrder(overlay.suitePackages)

loop:
	for {
		var cr gotestrunner.CompileResult
		var ok bool
		select {
		case cr, ok = <-compileCh:
			if !ok {
				break loop
			}
		case <-streamCtx.Done():
			break loop
		}

		singleCompiled := []gotestrunner.CompileResult{cr}
		singleSuites := map[string][]string{cr.Package: overlay.suitesByPkg[cr.Package]}
		targets := gotestrunner.BuildSuiteTargets(singleCompiled, singleSuites, overlay.dirsByPkg, pf.runFlags, pf.userRunFilter)

		if len(targets) == 0 {
			continue
		}
		anyTargets = true

		for j := range targets {
			targets[j].BudgetFile = targets[j].BinaryPath + ".budget"
		}

		if pf.userCoverProfile != "" {
			baseIdx := len(allTargets)
			for j := range targets {
				targets[j].CoverProfile = filepath.Join(coverDir, fmt.Sprintf("%d.out", baseIdx+j))
			}
			allTargets = append(allTargets, targets...)
		}

		collector.Register(cr.Package, len(targets))

		for i, target := range targets {
			wg.Add(1)
			go func(t gotestrunner.SuiteTarget, idx int) {
				defer wg.Done()

				requiredKeys := overlay.suiteRequiredSharedFixtureKeys[t.Package][t.SuiteName]
				var env []string
				if len(requiredKeys) > 0 {
					// Wait for the subprocess to start.
					select {
					case <-fixtureStarted:
					case <-streamCtx.Done():
						return
					}
					if fixtureStartErr != nil {
						return
					}

					// Wait for each required fixture to become ready.
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

					// Write a per-suite state file with only the required keys.
					stateFile, err := setupProc.WriteStateFileForKeys(t.SuiteName, requiredKeys)
					if err != nil {
						fmt.Fprintf(os.Stderr, "WARN: write state file for %s: %s\n", t.SuiteName, err)
						return
					}

					env = make([]string, len(baseEnv), len(baseEnv)+1)
					copy(env, baseEnv)
					env = append(env, "GOTEST_SHARED_STATE_FILE="+stateFile)
				} else {
					env = baseEnv
				}

				select {
				case sem <- struct{}{}:
				case <-streamCtx.Done():
					return
				}
				defer func() { <-sem }()

				r := gotestrunner.RunSingleSuite(streamCtx, t, env, collector.UsesTest2JSON())
				collector.RecordResult(t.Package, idx, r)
			}(target, i)
		}
	}

	wg.Wait()

	if setupProc != nil {
		setupProc.Teardown()
	}

	if pf.userCoverProfile != "" {
		mergeCoverProfiles(allTargets, pf.userCoverProfile)
	}

	if !anyTargets && len(overlay.noSuitePackages) == 0 {
		if !cfg.JSON {
			fmt.Fprintln(os.Stderr, "no test suites to run")
		}
		return 0, nil
	}

	collector.Finalize(overlay.noSuitePackages)

	return collector.WorstExitCode(), nil
}

func executeTestsCaptured(ctx context.Context, cfg ExecConfig, overlay *overlayResult) ([]byte, int, error) {
	pf := parseExecFlags(cfg.GoTestArgs)

	compiled, setupProc, err := prepareTestRun(ctx, overlay, pf.buildFlags, cfg.SetupTimeout)
	if err != nil {
		return nil, 2, err
	}
	if setupProc != nil {
		defer setupProc.Teardown()
	}

	select {
	case <-ctx.Done():
		return nil, 130, nil
	default:
	}

	extraEnv := buildExtraEnv(cfg, setupProc)
	runFlags := append(pf.runFlags, "-v")
	targets := gotestrunner.BuildSuiteTargets(compiled, overlay.suitesByPkg, overlay.dirsByPkg, runFlags, pf.userRunFilter)

	if len(targets) == 0 {
		return nil, 0, nil
	}

	for j := range targets {
		targets[j].BudgetFile = targets[j].BinaryPath + ".budget"
	}

	if pf.userCoverProfile != "" {
		coverDir := filepath.Join(overlay.tmpDir, "cover")
		os.MkdirAll(coverDir, 0o755)
		assignCoverProfiles(targets, coverDir)
		defer mergeCoverProfiles(targets, pf.userCoverProfile)
	}

	collector := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false)
	gotestrunner.RunSuites(ctx, targets, extraEnv, 0, collector)
	return collector.CapturedJSON(), collector.WorstExitCode(), nil
}

func Run(cfg ExecConfig) int {
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripCoverBuildFlags(classified.BuildFlags)
	loaded, err := gotestgen.LoadPackages(cfg.PackagePatterns, loadFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	if cfg.CI {
		if code, err := enforceFocusGuard(loaded); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		} else if code != 0 {
			return code
		}
	}

	overlay, cleanup, err := generateOverlayFromLoaded(loaded, cfg.Debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(),
		shutdownSignals...)
	defer stop()

	code, execErr := executeTestsStreaming(ctx, cfg, overlay)
	if execErr != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", execErr)
		return 2
	}
	return code
}

