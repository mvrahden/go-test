package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

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

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, cfg.Debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(),
		shutdownSignals...)
	defer stop()

	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		JSON:            cfg.JSON,
		UpdateSnapshots: cfg.UpdateSnapshots,
		Parallel:        cfg.Parallel,
		Streaming:       true,
		OutputMode:      modeFromJSON(cfg.JSON),
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return result.ExitCode
}

func modeFromJSON(jsonMode bool) gotestrunner.RunMode {
	if jsonMode {
		return gotestrunner.RunStreamJSON
	}
	return gotestrunner.RunBatchText
}
