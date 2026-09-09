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
	if sub, rest, ok := extractFuzzSubcommand(inv.Args); ok {
		switch sub {
		case "triage":
			return runFuzzTriage(rest)
		case "promote":
			return runFuzzPromote(rest)
		}
	}
	if w := misplacedFuzzSubcommand(inv.Args); w != "" {
		fmt.Fprintf(os.Stderr, "FAIL: the %s subcommand must come immediately after fuzz: gotest fuzz %s [packages...]\n", w, w)
		return 2
	}

	if extractStringFlag(inv.DefaultArgs(), "--timeout", "") != "" {
		fmt.Fprintln(os.Stderr, "FAIL: --timeout is not a fuzz flag: --for is the session's clock and the deadline follows it (--for=0 fuzzes until interrupted)")
		return 2
	}
	ownArgs, goTestArgs, err := SplitArgs(inv.DefaultArgs(), fuzzAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	forDuration, forExplicit, err := parseForFlag(ownArgs)
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

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, cfg.Debug, cfg.NoCache, cfg.HarvestSeeds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	targets := collectFuzzTargets(overlay)
	if name := extractStringFlag(ownArgs, "--target", ""); name != "" {
		targets, err = selectFuzzTargets(targets, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
	}
	if len(targets) == 0 {
		fmt.Println("no fuzz targets found")
		return 0
	}

	// Before spending any of the budget: a corpus entry left over from an
	// older shape of the fuzzed type aborts its target's run seconds in, and
	// the engine's own message names only the generated wrapper.
	gotestrunner.ReportStaleFuzzCorporaFor(os.Stderr, overlay, targets)

	// Print the schedule before spending any of the budget.
	session := planFuzzSession(forExplicit, forDuration, len(targets), jobs)
	budget, plan := session.Budget, session.Plan
	if budget > 0 {
		origin := ""
		if session.DefaultedBudget {
			origin = fmt.Sprintf("; --for defaulted to %s, --for=0 removes the budget", budget)
		}
		fmt.Fprintf(os.Stderr, "fuzzing %d target(s), %d at a time, %s each (~%s wall-clock, hard stop at %s%s)\n",
			plan.Targets, plan.Jobs, plan.PerTarget, plan.EstWall, session.Deadline, origin)
		if plan.Floored {
			fmt.Fprintf(os.Stderr, "note: the 10s per-target floor stretches this run past --for=%s\n", budget)
		}
	} else {
		fmt.Fprintf(os.Stderr, "fuzzing %d target(s), %d at a time, no budget: until interrupted\n", plan.Targets, plan.Jobs)
		if plan.Targets > plan.Jobs {
			fmt.Fprintf(os.Stderr, "note: without a budget only the first %d target(s) run; pass --for=<dur> to share time across all %d\n",
				plan.Jobs, plan.Targets)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer stop()

	if session.Deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, session.Deadline)
		defer cancel()
	}

	sessionStart := time.Now()
	res := gotestrunner.RunFuzzTargets(ctx, targets, gotestrunner.FuzzRunConfig{
		OverlayFlag: overlay.OverlayFlag,
		Total:       budget,
		Jobs:        jobs,
		BuildFlags:  classified.BuildFlags,
	})
	code := res.ExitCode()

	if err := ctx.Err(); err != nil {
		reason := "interrupted"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = fmt.Sprintf("deadline (%s) reached", session.Deadline)
		}
		if cut := res.CutShort(); budget > 0 && len(cut) > 0 {
			// An explicit --for budget was not honored — say which targets
			// lost time, so the shortfall is visible in CI logs even though
			// it is not by itself a failure.
			fmt.Fprintf(os.Stderr, "%s: %d target(s) did not get their full --for share: %s\n",
				reason, len(cut), strings.Join(cut, ", "))
		} else if code == 0 {
			fmt.Fprintf(os.Stderr, "%s; no failures found\n", reason)
		}
	}

	wall := time.Since(sessionStart)
	fmt.Fprintln(os.Stderr, fuzzSessionLine(res, wall))
	// Mirror gotest summary/bench: under GitHub Actions the session lands
	// in the job summary beside the test results.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		if summaryPath := os.Getenv("GITHUB_STEP_SUMMARY"); summaryPath != "" {
			if sf, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				renderFuzzSessionMarkdown(sf, res, wall)
				sf.Close()
			}
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

// selectFuzzTargets narrows targets to the one generated wrapper --target
// names. An unmatched name is a usage error listing what exists — a typo
// (or a rename that outdated an editor invocation) must never silently
// fall back to fuzzing everything.
func selectFuzzTargets(targets []gotestrunner.FuzzTarget, name string) ([]gotestrunner.FuzzTarget, error) {
	names := make([]string, 0, len(targets))
	for i := range targets {
		if targets[i].Func == name {
			return targets[i : i+1 : i+1], nil
		}
		names = append(names, targets[i].Func)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no fuzz target named %q: the matched packages declare no fuzz targets", name)
	}
	return nil, fmt.Errorf("no fuzz target named %q; available: %s", name, strings.Join(names, ", "))
}

// defaultFuzzFor bounds a session that names no budget: a minute of
// wall-clock, shared jobs-aware across the targets. Unbounded runs starve
// every wave after the first, so they are the opt-in (--for=0), not the default.
const defaultFuzzFor = time.Minute

// parseForFlag reads --for. explicit reports whether the flag was given at
// all; --for=0 is the explicit opt-out, like --timeout=0.
func parseForFlag(args []string) (d time.Duration, explicit bool, err error) {
	raw := extractStringFlag(args, "--for", "")
	if raw == "" {
		return 0, false, nil
	}
	d, err = time.ParseDuration(raw)
	if err != nil {
		return 0, true, fmt.Errorf("invalid --for value %q: %w", raw, err)
	}
	if d < 0 {
		return 0, true, fmt.Errorf("invalid --for value %q: must be positive, or 0 for no budget", raw)
	}
	return d, true, nil
}

// fuzzSession is what --for resolves to: the budget the schedule splits
// (0 = none) and the deadline that follows it (0 = none).
type fuzzSession struct {
	Budget          time.Duration
	Deadline        time.Duration
	DefaultedBudget bool
	Plan            gotestrunner.FuzzSchedule
}

// planFuzzSession makes --for the session's one clock. Without it the
// default budget applies. The deadline follows the schedule plus headroom
// for generation, instrumented builds and minimisation; --for=0 runs until
// interrupted with no deadline at all. The pipeline's --timeout is refused
// by the fuzz command rather than left to compete.
func planFuzzSession(forExplicit bool, forDur time.Duration, targets, jobs int) fuzzSession {
	s := fuzzSession{Budget: defaultFuzzFor, DefaultedBudget: true}
	if forExplicit {
		s.Budget, s.DefaultedBudget = forDur, false
	}
	s.Plan = gotestrunner.PlanFuzzSchedule(s.Budget, targets, jobs)
	if s.Budget > 0 {
		s.Deadline = s.Plan.EstWall + max(2*time.Minute, s.Plan.EstWall/2)
	}
	return s
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
