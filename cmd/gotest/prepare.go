package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

type prepareOutput struct {
	OverlayFile string `json:"overlayFile"`
	Dir         string `json:"dir"`
	StateFile   string `json:"stateFile,omitempty"`
}

func runPrepare(args []string) int {
	patterns := ExtractPackagePatterns(args)

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	overlay, cleanup, err := generateOverlay(patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		setupProc, err = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures)
		if err != nil {
			cleanup()
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup: %s\n", err)
			return 2
		}
	}

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
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	if setupProc != nil {
		setupProc.Teardown()
	}
	cleanup()
	return 0
}
