package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

// prepareOutput is the document of "gotest prepare"; Version opens it.
type prepareOutput struct {
	Version     string `json:"version"`
	OverlayFile string `json:"overlayFile"`
	Dir         string `json:"dir"`
	StateFile   string `json:"stateFile,omitempty"`
}

func runPrepare(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	_, args, err := SplitArgs(inv.TagArgs(), noOwnFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	patterns, err := ExtractPackagePatterns(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	tags, _ := extractTagsFlag(args)
	var buildFlags []string
	if tags != "" {
		buildFlags = append(buildFlags, "-tags="+tags)
	}

	loaded, broken, err := gotestgen.LoadPackages(patterns, buildFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if reportBrokenPackages(broken) {
		return 2
	}

	ctx, stop := shutdownContext()

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, nil, false, false, prepareHarvestSeeds(inv.Config))
	if err != nil {
		stop()
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	var setupProc *gotestrunner.SharedFixtureProcess
	if len(overlay.SharedFixtures) > 0 {
		setupProc, err = gotestrunner.StartSharedFixtures(ctx, overlay.WorkDir, overlay.SharedFixtures, 0)
		if err != nil {
			stop()
			cleanup()
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup: %s\n", err)
			return 2
		}
		if err := setupProc.WaitAllReady(ctx, 0); err != nil {
			stop()
			_ = setupProc.Teardown()
			cleanup()
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup: %s\n", err)
			return 2
		}
	}

	out := prepareOutput{
		Version:     about.ResolvedVersion(),
		OverlayFile: filepath.Join(overlay.CacheDir, "overlay.json"),
		Dir:         overlay.WorkDir,
	}
	if setupProc != nil {
		out.StateFile = setupProc.StateFile()
	}

	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		stop()
		if setupProc != nil {
			_ = setupProc.Teardown()
		}
		cleanup()
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	// The handler stays installed from setup to shutdown: a signal that lands
	// right after the JSON line must still tear the fixtures down.
	<-ctx.Done()
	stop()

	code := 0
	if setupProc != nil {
		// A fixture that fails to release what it held is a real failure
		// here as in a run: the editor that asked for the fixtures must
		// learn that something was left behind.
		if err := setupProc.Teardown(); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture teardown: %s\n", err)
			code = 1
		}
	}
	cleanup()
	return code
}

// prepareHarvestSeeds mirrors a run: the project's fuzz.harvest setting
// decides whether the overlay carries harvested seeds.
func prepareHarvestSeeds(cfg config.ProjectConfig) bool { //nolint:gocritic // hugeParam: stable API
	return cfg.Fuzz.HarvestSeeds()
}
