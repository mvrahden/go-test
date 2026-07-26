package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// runFuzz discovers FuzzX suite methods and drives each generated
// Fuzz<Suite>_<Method> wrapper as its own "go test -fuzz=..." process via
// gotestrunner.RunFuzzTargets. Unlike runTest/runBench, it does not run the
// shared compiled suite binary — see the doc comment on
// internal/gotestrunner/fuzzrun.go for why.
func runFuzz(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	if sub, rest, ok := extractFuzzSubcommand(inv.Args); ok {
		switch sub {
		case "triage":
			return runFuzzTriage(rest)
		case "promote":
			return runFuzzPromote(rest)
		}
	}

	ownArgs, goTestArgs, err := SplitArgs(inv.DefaultArgs(), fuzzAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	forDuration, err := parseForFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	jobs, err := parseJobsFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	cfg, err := parseExecFlags(ownArgs, goTestArgs, &inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripCoverBuildFlags(classified.BuildFlags)
	loaded, err := gotestgen.LoadPackages(cfg.PackagePatterns, loadFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, cfg.Debug, cfg.NoCache, cfg.HarvestSeeds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	targets := collectFuzzTargets(overlay)
	if len(targets) == 0 {
		fmt.Println("no fuzz targets found")
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer stop()

	if cfg.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer cancel()
	}

	code := gotestrunner.RunFuzzTargets(ctx, targets, gotestrunner.FuzzRunConfig{
		OverlayFlag: overlay.OverlayFlag,
		Total:       forDuration,
		Jobs:        jobs,
		BuildFlags:  classified.BuildFlags,
	})

	if cfg.GlobalTimeout > 0 && ctx.Err() == context.DeadlineExceeded && code == 0 {
		code = 1
	}

	return code
}

// collectFuzzTargets flattens overlay.FuzzFuncsByPkg (import path -> suite
// struct name -> generated Fuzz<Suite>_<Method> func names) into a
// deterministically ordered slice of FuzzTarget, pairing each with its
// package's source directory from overlay.DirsByPkg.
func collectFuzzTargets(overlay *gotestrunner.OverlayResult) []gotestrunner.FuzzTarget {
	var targets []gotestrunner.FuzzTarget
	for pkg, bySuite := range overlay.FuzzFuncsByPkg {
		dir := overlay.DirsByPkg[pkg]
		for _, funcs := range bySuite {
			for _, fn := range funcs {
				targets = append(targets, gotestrunner.FuzzTarget{
					Package: pkg,
					Dir:     dir,
					Func:    fn,
				})
			}
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Package != targets[j].Package {
			return targets[i].Package < targets[j].Package
		}
		return targets[i].Func < targets[j].Func
	})
	return targets
}

func parseForFlag(args []string) (time.Duration, error) {
	raw := extractStringFlag(args, "--for", "")
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --for value %q: %w", raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid --for value %q: must be positive", raw)
	}
	return d, nil
}

func parseJobsFlag(args []string) (int, error) {
	raw := extractStringFlag(args, "--jobs", "")
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --jobs value %q: must be a positive integer", raw)
	}
	if v <= 0 {
		return 0, fmt.Errorf("invalid --jobs value %d: must be positive", v)
	}
	return v, nil
}
