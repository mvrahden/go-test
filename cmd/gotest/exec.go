package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

func Run(cfg ExecConfig) int { //nolint:gocritic // hugeParam: stable API
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripCoverBuildFlags(classified.BuildFlags)
	loaded, broken, err := gotestgen.LoadPackages(cfg.PackagePatterns, loadFlags)
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

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, cfg.Debug, cfg.NoCache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(),
		shutdownSignals...)
	defer stop()

	if cfg.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer cancel()
	}

	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		Streaming:       true,
		OutputMode:      modeFromJSON(cfg.JSON),
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if cfg.GlobalTimeout > 0 && ctx.Err() == context.DeadlineExceeded {
		fmt.Fprintf(os.Stderr, "FAIL: global --timeout exceeded after %v\n", cfg.GlobalTimeout)
		if result.ExitCode == 0 {
			return 1
		}
	}
	if total, names := sumStdlibTests(overlay.StdlibTestsByPkg); total > 0 && !cfg.JSON {
		fmt.Fprintf(os.Stderr, "note: %d stdlib test(s) in %s not run — gotest runs suites; use 'go test' for stdlib tests\n", total, names)
	}
	return result.ExitCode
}

// sumStdlibTests totals the stdlib tests gotest reports but does not run, and
// names the affected packages (truncated) so the note is actionable.
func sumStdlibTests(byPkg map[string]int) (int, string) {
	total := 0
	pkgs := make([]string, 0, len(byPkg))
	for pkg, n := range byPkg {
		total += n
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	if len(pkgs) > 3 {
		pkgs = append(pkgs[:3], fmt.Sprintf("… %d more", len(byPkg)-3))
	}
	return total, strings.Join(pkgs, ", ")
}

func modeFromJSON(jsonMode bool) gotestrunner.RunMode {
	if jsonMode {
		return gotestrunner.RunStreamJSON
	}
	return gotestrunner.RunBatchText
}
