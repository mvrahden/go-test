package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/inputhash"
	"github.com/mvrahden/go-test/internal/protocol"
)

// processStart is when this invocation began: a run's start.
var processStart = time.Now()

// runInputs are the input hashes of a run's packages, computed beside the
// run from the graph its load listed.
type runInputs struct {
	flags  []string
	done   chan struct{}
	hashes inputhash.Hashes
}

// startInputs hashes the loaded packages under the run's build flags.
func startInputs(pkgs *gotestgen.RunPackages, buildFlags []string) *runInputs {
	in := &runInputs{flags: inputhash.Flags(buildFlags), done: make(chan struct{})}
	graph := pkgs.Graph
	go func() {
		defer close(in.done)
		settings, err := inputhash.ReadSettings(context.Background(), "", buildFlags)
		if err != nil {
			return
		}
		in.hashes = inputhash.Packages(graph, settings)
	}()
	return in
}

// all waits for the hashes; nil when they could not be made.
func (in *runInputs) all() inputhash.Hashes {
	if in == nil {
		return nil
	}
	<-in.done
	return in.hashes
}

// of is pkg's hash, "" for none.
func (in *runInputs) of(pkg string) string { return in.all()[pkg] }

// stamp is the pipeline's view of the hashes; nil without them.
func (in *runInputs) stamp() func(string) string {
	if in == nil {
		return nil
	}
	return in.of
}

// runPlan is what a run decided before it ran beside its config: its
// packages' hashes and how --changed narrowed it.
type runPlan struct {
	inputs  *runInputs
	changed *protocol.Changed
	// merge keeps in the record the packages the run left out.
	merge bool
	// only is the selection of a --changed --failed run; nil for none.
	only map[string]string
	// mode is the mode the run's verdicts are taken under.
	mode protocol.VerdictMode
}

// planRun hashes the loaded packages and, under --changed, keeps only
// those whose verdict the checkout's record does not hold as current:
// their hash differs, or the record holds no verdict, a failed one or one
// of a part of their tests. It reports false when it said on stderr why
// the run cannot go on.
func planRun(cfg *ExecConfig, pkgs *gotestgen.RunPackages) (*runPlan, bool) {
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	plan := &runPlan{inputs: startInputs(pkgs, classified.BuildFlags), mode: runMode(cfg)}
	if cfg.Manifest != nil {
		cfg.Manifest.Build = plan.inputs.flags
	}
	if !cfg.Changed {
		return plan, true
	}
	plan.merge = true
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: --changed: %s\n", err)
		return nil, false
	}
	rec, err := gotestrunner.ReadRunRecord(dir)
	if errors.Is(err, os.ErrNotExist) || err == nil && rec.Packages == nil {
		fmt.Fprintf(os.Stderr, "gotest: --changed: no run of %s is recorded, so every package runs\n", dir)
		plan.changed = narrowedTo(pkgs.Order, 0)
		plan.changed.NoRecord = true
		plan.setManifest(cfg)
		return plan, true
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: --changed: reading the last run's record: %s\n", err)
		return nil, false
	}
	hashes := plan.inputs.all()
	recorded := map[string]protocol.PackageVerdict{}
	for _, p := range rec.Packages {
		recorded[p.Package] = p
	}
	failedOnly := map[string]bool{}
	keep := map[string]bool{}
	otherMode := 0
	for _, pkg := range pkgs.Order {
		r := recorded[pkg]
		current, other := heldCurrent(r, hashes[pkg], plan.mode)
		if other {
			otherMode++
		}
		switch {
		case !current:
			keep[pkg] = true
		case r.Verdict == "fail":
			keep[pkg], failedOnly[pkg] = true, true
		}
	}
	if otherMode > 0 {
		fmt.Fprintf(os.Stderr, "gotest: --changed: %d unchanged package(s) run again: the record does not hold their verdict under this run's -short and --ci\n", otherMode)
	}
	if cfg.ChangedFailed {
		only := gotestrunner.RerunSelection(rec.Failed)
		cfg.Only = map[string]string{}
		for pkg := range keep {
			sel, failed := only[pkg]
			if failedOnly[pkg] && failed {
				cfg.Only[pkg] = sel
			} else {
				cfg.Only[pkg] = ""
			}
		}
		plan.only = cfg.Only
	}
	unchanged := len(pkgs.Order) - len(keep)
	narrowPackages(pkgs, keep)
	plan.changed = narrowedTo(pkgs.Order, unchanged)
	plan.setManifest(cfg)
	return plan, true
}

// narrowedTo is the narrowing of a run that runs pkgs and left out
// unchanged packages.
func narrowedTo(pkgs []string, unchanged int) *protocol.Changed {
	sorted := slices.Sorted(slices.Values(pkgs))
	if sorted == nil {
		sorted = []string{}
	}
	return &protocol.Changed{Ran: len(sorted), Unchanged: unchanged, Packages: sorted}
}

// planReport is planRun for spec and summary, whose run always opens a
// stream with its manifest.
func planReport(cfg *ExecConfig, pkgs *gotestgen.RunPackages) (*runPlan, bool) {
	cfg.Manifest = runManifest()
	return planRun(cfg, pkgs)
}

func (p *runPlan) setManifest(cfg *ExecConfig) {
	if cfg.Manifest != nil {
		cfg.Manifest.Changed = p.changed
	}
}

// narrowPackages keeps of pkgs those keep names.
func narrowPackages(pkgs *gotestgen.RunPackages, keep map[string]bool) {
	pkgs.Loaded = slices.DeleteFunc(pkgs.Loaded, func(lr *gotestgen.LoadResult) bool { return !keep[lr.PkgPath] })
	pkgs.Broken = slices.DeleteFunc(pkgs.Broken, func(b gotestgen.BrokenPackage) bool { return !keep[b.PkgPath] })
	pkgs.NoTestFiles = slices.DeleteFunc(pkgs.NoTestFiles, func(p string) bool { return !keep[p] })
	pkgs.Order = slices.DeleteFunc(pkgs.Order, func(p string) bool { return !keep[p] })
}

// recordOptions are what a run's record holds beside what the pipeline
// judged.
type recordOptions struct {
	plan       *runPlan
	eventsFile string
	// narrowed reports whether a package ran a part of its tests.
	narrowed func(pkg string) bool
}

// narrowedBy is the narrowing a run's flags make of each package: -run,
// -skip, --group and --skip-group narrow all, --failed's selection the
// packages it narrows.
func narrowedBy(cfg *ExecConfig, plan *runPlan) func(string) bool {
	all := hasFlag(cfg.GoTestArgs, "-run") || hasFlag(cfg.GoTestArgs, "-skip") || len(cfg.Groups)+len(cfg.SkipGroups) > 0
	only := cfg.Only
	if plan != nil && plan.only != nil {
		only = plan.only
	}
	return func(pkg string) bool {
		if all {
			return true
		}
		sel, ok := only[pkg]
		return ok && sel != ""
	}
}

// saveRun leaves the run's record for --failed and --changed, and in the
// checkout's history; it says on stderr when it cannot.
func saveRun(result *gotestrunner.PipelineResult, code int, manifest *protocol.Manifest, opts recordOptions) []protocol.FailedUnit {
	if result == nil || result.Judged == nil || manifest == nil {
		return nil
	}
	if os.Getenv(protocol.EnvMutantOverlay) != "" {
		return result.Failed // a run on audit --mutate's mutants is not of the tree
	}
	dir, err := os.Getwd()
	if err != nil {
		return result.Failed
	}
	res := *result.Judged
	res.ExitCode = code
	rec := protocol.RunRecord{Dir: dir, Manifest: *manifest, Result: res, Failed: result.Failed, Packages: slices.Clone(result.Packages), EventsFile: absPath(opts.eventsFile), Started: processStart.UTC()}
	var mode protocol.VerdictMode
	if opts.plan != nil {
		mode = opts.plan.mode
	}
	stampRecord(&rec, mode, opts.narrowed, false)
	merge := opts.plan != nil && opts.plan.merge
	if err := gotestrunner.SaveRun(rec, merge, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "gotest: the run is not recorded for --failed: %s\n", err)
	}
	return result.Failed
}

func absPath(path string) string {
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
