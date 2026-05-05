package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

type overlayResult struct {
	tmpDir         string
	overlayFlag    string
	sharedFixtures []gotestgen.SharedFixtureInfo
}

func generateOverlay(patterns []string) (*overlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateWithSharedFixtures(patterns, nil)
	if err != nil {
		return nil, nil, err
	}

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {}
	if DEBUG {
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s\n", tmpDir)
	} else {
		cleanup = func() { os.RemoveAll(tmpDir) }
	}

	return &overlayResult{
		tmpDir:         tmpDir,
		overlayFlag:    "-overlay=" + filepath.Join(tmpDir, "overlay.json"),
		sharedFixtures: allSharedFixtures,
	}, cleanup, nil
}

func buildExtraEnv() map[string]string {
	env := make(map[string]string)
	if UPDATE_SNAPSHOTS {
		env["GOTEST_UPDATE_SNAPSHOTS"] = "1"
	}
	return env
}

func Run(cfg ExecConfig) int {
	if CI {
		violations, err := RunFocusGuard(cfg.PackagePatterns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		if len(violations) > 0 {
			fmt.Fprintln(os.Stderr, "FAIL: focus prefix detected — remove F_ before merging:")
			for _, v := range violations {
				fmt.Fprintln(os.Stderr, v.String())
			}
			return 1
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	overlay, cleanup, err := generateOverlay(cfg.PackagePatterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	goTestArgs := append([]string{overlay.overlayFlag}, cfg.GoTestArgs...)

	// If any shared fixtures, start setup subprocess
	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		var err error
		setupProc, err = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup: %s\n", err)
			return 2
		}
		defer setupProc.Teardown()
	}

	select {
	case <-ctx.Done():
		return 130
	default:
	}

	extraEnv := buildExtraEnv()
	if setupProc != nil {
		extraEnv["GOTEST_SHARED_STATE_FILE"] = setupProc.StateFile()
	}

	if SPEC {
		return runWithSpec(ctx, goTestArgs, extraEnv)
	}

	code, err := gotestrunner.StdlibRunTests(ctx, goTestArgs, extraEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return code
}

func runWithSpec(ctx context.Context, goTestArgs []string, extraEnv map[string]string) int {
	jsonData, code, err := gotestrunner.StdlibRunTestsJSON(ctx, goTestArgs, extraEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}

	// Replay normal output
	for _, ev := range events {
		if ev.Output != "" {
			fmt.Print(ev.Output)
		}
	}

	// Render spec view
	fmt.Println()
	tree := gotestspec.BuildTree(events)
	gotestspec.RenderTerminal(os.Stdout, tree)

	return code
}
