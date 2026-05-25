package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

type prepareOutput struct {
	OverlayFile string `json:"overlayFile"`
	Dir         string `json:"dir"`
	StateFile   string `json:"stateFile,omitempty"`
}

func runPrepare(inv Invocation) int {
	args := inv.TagArgs()
	patterns := ExtractPackagePatterns(args)
	tags, _ := extractTagsFlag(args)
	var buildFlags []string
	if tags != "" {
		buildFlags = append(buildFlags, "-tags="+tags)
	}

	loaded, err := gotestgen.LoadPackages(patterns, buildFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		shutdownSignals...)

	overlay, cleanup, err := generateOverlayFromLoaded(loaded, false)
	if err != nil {
		stop()
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		setupProc, err = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures, 0)
		if err != nil {
			stop()
			cleanup()
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup: %s\n", err)
			return 2
		}
	}

	// Stop the context-based listener before switching to channel-based blocking.
	stop()

	out := prepareOutput{
		OverlayFile: filepath.Join(overlay.tmpDir, "overlay.json"),
		Dir:         overlay.tmpDir,
	}
	if setupProc != nil {
		out.StateFile = setupProc.StateFile()
	}

	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		if setupProc != nil {
			setupProc.Teardown()
		}
		cleanup()
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, shutdownSignals...)
	<-sigCh

	if setupProc != nil {
		setupProc.Teardown()
	}
	cleanup()
	return 0
}
