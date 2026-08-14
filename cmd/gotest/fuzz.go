package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// runFuzz discovers FuzzX suite methods and drives each generated
// Fuzz<Suite>_<Method> wrapper as its own "go test -fuzz=..." process via
// gotestrunner.RunFuzzTargets. Unlike runTest/runBench, it does not run the
// shared compiled suite binary — see the doc comment on
// internal/gotestrunner/fuzzrun.go for why.
//
// Exit contract: 0 = the search ran and found nothing — including sessions
// ended by the global --timeout or an interrupt, which is the normal way an
// open-ended run (no --for) terminates; 1 = a finding (a failing target or
// a new crasher); 2 = the session could not run as requested (bad flags,
// broken packages). Time exhaustion is the expected end of a search, never
// a failure by itself.
func runFuzz(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
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
	loaded, broken, err := gotestgen.LoadPackages(cfg.PackagePatterns, loadFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	// Fuzzing a partially built tree proves nothing: fail fast like
	// generate/prepare rather than book-and-continue like run.
	if reportBrokenPackages(broken) {
		return 2
	}

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, cfg.Debug, cfg.NoCache)
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

	// Print the schedule the budget resolves to before spending any of it,
	// and reject an impossible --for/--timeout combination here rather than
	// letting the deadline cut the run down mid-flight.
	plan := gotestrunner.PlanFuzzSchedule(forDuration, len(targets), jobs)
	if forDuration > 0 {
		if cfg.GlobalTimeout > 0 && plan.EstWall >= cfg.GlobalTimeout {
			fmt.Fprintf(os.Stderr, "FAIL: --for=%s needs ~%s of fuzzing wall-clock (%d target(s), %d at a time) but the global --timeout is %s — raise --timeout, lower --for, or pass --timeout=0\n",
				forDuration, plan.EstWall, plan.Targets, plan.Jobs, cfg.GlobalTimeout)
			return 2
		}
		fmt.Fprintf(os.Stderr, "fuzzing %d target(s), %d at a time, %s each (~%s wall-clock)\n",
			plan.Targets, plan.Jobs, plan.PerTarget, plan.EstWall)
		if plan.Floored {
			fmt.Fprintf(os.Stderr, "note: the 10s per-target floor stretches this run past --for=%s\n", forDuration)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer stop()

	if cfg.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer cancel()
	}

	res := gotestrunner.RunFuzzTargets(ctx, targets, gotestrunner.FuzzRunConfig{
		OverlayFlag: overlay.OverlayFlag,
		Total:       forDuration,
		Jobs:        jobs,
		BuildFlags:  classified.BuildFlags,
	})
	code := res.ExitCode()

	if err := ctx.Err(); err != nil {
		reason := "interrupted"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = fmt.Sprintf("global --timeout (%s) reached", cfg.GlobalTimeout)
		}
		if cut := res.CutShort(); forDuration > 0 && len(cut) > 0 {
			// An explicit --for budget was not honored — say which targets
			// lost time, so the shortfall is visible in CI logs even though
			// it is not by itself a failure.
			fmt.Fprintf(os.Stderr, "%s: %d target(s) did not get their full --for share: %s\n",
				reason, len(cut), strings.Join(cut, ", "))
		} else if code == 0 {
			fmt.Fprintf(os.Stderr, "%s; no failures found\n", reason)
		}
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
