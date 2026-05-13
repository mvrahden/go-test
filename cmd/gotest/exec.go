package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

type overlayResult struct {
	tmpDir           string
	overlayFlag      string
	sharedFixtures   []gotestgen.SharedFixtureInfo
	suitePackages    []string
	suitesByPkg      map[string][]string          // import path → suite struct names
	fixtureDepSuites map[string]map[string]bool   // import path → set of test func names needing shared fixtures
}

func generateOverlayFromLoaded(loaded []*gotestgen.LoadResult, debug bool) (*overlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateFromLoaded(loaded)
	if err != nil {
		return nil, nil, err
	}

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
	suitesByPkg := map[string][]string{}
	fixtureDepSuites := map[string]map[string]bool{}
	for _, r := range allResults {
		if len(r.PTest) > 0 || len(r.PXTest) > 0 {
			suitePkgs = append(suitePkgs, r.PkgPath)
		}
		if len(r.SuiteNames) > 0 {
			suitesByPkg[r.PkgPath] = r.SuiteNames
		}
		if len(r.FixtureDepSuites) > 0 {
			s := make(map[string]bool, len(r.FixtureDepSuites))
			for _, fn := range r.FixtureDepSuites {
				s[fn] = true
			}
			fixtureDepSuites[r.PkgPath] = s
		}
	}

	return &overlayResult{
		tmpDir:           tmpDir,
		overlayFlag:      "-overlay=" + filepath.Join(tmpDir, "overlay.json"),
		sharedFixtures:   allSharedFixtures,
		suitePackages:    suitePkgs,
		suitesByPkg:      suitesByPkg,
		fixtureDepSuites: fixtureDepSuites,
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
	buildFlags      []string
	runFlags        []string
	userRunFilter   string
	userCoverProfile string
}

func parseExecFlags(goTestArgs []string) parsedFlags {
	classified := gotestrunner.ClassifyGoTestArgs(goTestArgs)
	userRunFilter := gotestrunner.ExtractRunFilter(classified.RunFlags)
	runFlags := gotestrunner.StripRunFilter(classified.RunFlags)
	userCoverProfile := gotestrunner.ExtractCoverProfile(runFlags)
	runFlags = gotestrunner.StripCoverProfile(runFlags)
	return parsedFlags{
		buildFlags:       classified.BuildFlags,
		runFlags:         runFlags,
		userRunFilter:    userRunFilter,
		userCoverProfile: userCoverProfile,
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
	targets := gotestrunner.BuildSuiteTargets(compiled, overlay.suitesByPkg, pf.runFlags, pf.userRunFilter)

	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "no test suites to run")
		return 0, nil
	}

	setupCoverage(targets, overlay, pf.userCoverProfile)
	if pf.userCoverProfile != "" {
		defer mergeCoverProfiles(targets, pf.userCoverProfile)
	}

	if cfg.Spec {
		return runWithSpec(ctx, targets, extraEnv), nil
	}

	if cfg.JSON {
		_, code := gotestrunner.RunSuitesTest2JSON(ctx, targets, extraEnv, 0)
		return code, nil
	}

	_, code := gotestrunner.RunSuites(ctx, targets, extraEnv, 0)
	return code, nil
}

func executeTestsStreaming(ctx context.Context, cfg ExecConfig, overlay *overlayResult) (int, error) {
	pf := parseExecFlags(cfg.GoTestArgs)

	var coverDir string
	if pf.userCoverProfile != "" {
		coverDir = filepath.Join(overlay.tmpDir, "cover")
		os.MkdirAll(coverDir, 0o755)
	}

	baseEnv := os.Environ()
	if cfg.UpdateSnapshots {
		baseEnv = append(baseEnv, "GOTEST_UPDATE_SNAPSHOTS=1")
	}

	// Start fixture setup and compilation concurrently.
	fixtureReady := make(chan struct{})
	var setupProc *SharedFixtureProcess
	var setupErr error

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	if len(overlay.sharedFixtures) > 0 {
		go func() {
			setupProc, setupErr = startSharedFixtures(streamCtx, overlay.tmpDir, overlay.sharedFixtures, cfg.SetupTimeout)
			close(fixtureReady)
		}()
	} else {
		close(fixtureReady)
	}

	compileCh := gotestrunner.CompilePackagesStream(streamCtx, overlay.suitePackages, overlay.overlayFlag, pf.buildFlags, overlay.tmpDir)

	maxParallel := 2 * runtime.GOMAXPROCS(0)
	sem := make(chan struct{}, maxParallel)
	var mu sync.Mutex
	var wg sync.WaitGroup
	worstCode := 0
	anyTargets := false
	var allTargets []gotestrunner.SuiteTarget

	// Lazy fixture env resolution — first caller blocks until fixtures are ready.
	var fixtureEnv []string
	var fixtureEnvErr error
	var fixtureOnce sync.Once

	resolveFixtureEnv := func() ([]string, error) {
		fixtureOnce.Do(func() {
			select {
			case <-fixtureReady:
			case <-streamCtx.Done():
				fixtureEnvErr = streamCtx.Err()
				return
			}
			if setupErr != nil {
				fixtureEnvErr = fmt.Errorf("shared fixture setup: %w", setupErr)
				streamCancel()
				return
			}
			fixtureEnv = make([]string, len(baseEnv))
			copy(fixtureEnv, baseEnv)
			if setupProc != nil {
				fixtureEnv = append(fixtureEnv, "GOTEST_SHARED_STATE_FILE="+setupProc.StateFile())
			}
		})
		return fixtureEnv, fixtureEnvErr
	}

	// Per-package state for CLI output batching (non-JSON mode).
	// Results are stored at their target index to preserve deterministic ordering.
	type pkgState struct {
		expected  int
		completed int
		results   []gotestrunner.SuiteResult
	}
	pkgStates := map[string]*pkgState{}

loop:
	for cr := range compileCh {
		select {
		case <-streamCtx.Done():
			break loop
		default:
		}

		singleCompiled := []gotestrunner.CompileResult{cr}
		singleSuites := map[string][]string{cr.Package: overlay.suitesByPkg[cr.Package]}
		targets := gotestrunner.BuildSuiteTargets(singleCompiled, singleSuites, pf.runFlags, pf.userRunFilter)

		if len(targets) == 0 {
			continue
		}
		anyTargets = true

		if pf.userCoverProfile != "" {
			baseIdx := len(allTargets)
			for j := range targets {
				targets[j].CoverProfile = filepath.Join(coverDir, fmt.Sprintf("%d.out", baseIdx+j))
			}
			allTargets = append(allTargets, targets...)
		}

		if !cfg.JSON {
			mu.Lock()
			pkgStates[cr.Package] = &pkgState{
				expected: len(targets),
				results:  make([]gotestrunner.SuiteResult, len(targets)),
			}
			mu.Unlock()
		}

		for i, target := range targets {
			wg.Add(1)
			go func(t gotestrunner.SuiteTarget, idx int) {
				defer wg.Done()

				needsFixture := overlay.fixtureDepSuites[t.Package][t.SuiteName]
				var env []string
				if needsFixture {
					var err error
					env, err = resolveFixtureEnv()
					if err != nil {
						return
					}
				} else {
					env = baseEnv
				}

				sem <- struct{}{}
				defer func() { <-sem }()

				mu.Lock()
				if streamCtx.Err() != nil {
					mu.Unlock()
					return
				}
				mu.Unlock()

				if cfg.JSON {
					r := gotestrunner.RunSingleSuiteTest2JSON(streamCtx, t, env)
					mu.Lock()
					os.Stdout.Write(r.Stdout)
					if len(r.Stderr) > 0 {
						os.Stderr.Write(r.Stderr)
					}
					if r.ExitCode > worstCode {
						worstCode = r.ExitCode
					}
					mu.Unlock()
				} else {
					r := gotestrunner.RunSingleSuite(streamCtx, t, env)
					mu.Lock()
					s := pkgStates[t.Package]
					s.results[idx] = r
					s.completed++
					if r.ExitCode > worstCode {
						worstCode = r.ExitCode
					}
					if s.completed == s.expected {
						pkgFailed := false
						var pkgDuration time.Duration
						for _, pr := range s.results {
							os.Stdout.Write(gotestrunner.StripTrailingStatus(pr.Stdout))
							if len(pr.Stderr) > 0 {
								os.Stderr.Write(pr.Stderr)
							}
							if pr.ExitCode != 0 {
								pkgFailed = true
							}
							pkgDuration += pr.Duration
						}
						gotestrunner.WritePackageSummary(t.Package, pkgFailed, pkgDuration)
					}
					mu.Unlock()
				}
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

	if !anyTargets {
		if !cfg.JSON {
			fmt.Fprintln(os.Stderr, "no test suites to run")
		}
		return 0, nil
	}

	return worstCode, nil
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
	targets := gotestrunner.BuildSuiteTargets(compiled, overlay.suitesByPkg, runFlags, pf.userRunFilter)

	if len(targets) == 0 {
		return nil, 0, nil
	}

	if pf.userCoverProfile != "" {
		coverDir := filepath.Join(overlay.tmpDir, "cover")
		os.MkdirAll(coverDir, 0o755)
		assignCoverProfiles(targets, coverDir)
		defer mergeCoverProfiles(targets, pf.userCoverProfile)
	}

	data, code := gotestrunner.RunSuitesJSON(ctx, targets, extraEnv, 0)
	return data, code, nil
}

func Run(cfg ExecConfig) int {
	loaded, err := gotestgen.LoadPackages(cfg.PackagePatterns, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	if cfg.CI {
		suites, err := gotestgen.CollectFromLoaded(loaded)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		violations := CheckFocusViolations(suites)
		if len(violations) > 0 {
			fmt.Fprintln(os.Stderr, "FAIL: focus prefix detected — remove F_ before merging:")
			for _, v := range violations {
				fmt.Fprintln(os.Stderr, v.String())
			}
			return 1
		}
	}

	overlay, cleanup, err := generateOverlayFromLoaded(loaded, cfg.Debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	var code int
	var execErr error
	if cfg.Spec {
		code, execErr = executeTests(ctx, cfg, overlay)
	} else {
		code, execErr = executeTestsStreaming(ctx, cfg, overlay)
	}
	if execErr != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", execErr)
		return 2
	}
	return code
}

func runWithSpec(ctx context.Context, targets []gotestrunner.SuiteTarget, extraEnv map[string]string) int {
	// Ensure -test.v for JSON parsing
	for i := range targets {
		hasV := false
		for _, f := range targets[i].RunFlags {
			if f == "-test.v" {
				hasV = true
				break
			}
		}
		if !hasV {
			targets[i].RunFlags = append(targets[i].RunFlags, "-test.v")
		}
	}

	jsonData, code := gotestrunner.RunSuitesJSON(ctx, targets, extraEnv, 0)

	events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}

	for _, ev := range events {
		if ev.Output != "" {
			fmt.Print(ev.Output)
		}
	}

	fmt.Println()
	tree := gotestspec.BuildTree(events)
	gotestspec.RenderTerminal(os.Stdout, tree)

	return code
}
