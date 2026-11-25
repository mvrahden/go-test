package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/internal/inputhash"
)

// streamBuild is the build flags the run of events stamped its hashes
// under, as its manifest names them.
func streamBuild(events []gotestspec.TestEvent) []string {
	if run, ok := gotestspec.RunOf(events); ok {
		return run.Manifest.Build
	}
	return nil
}

// currentHashes are the input hashes the working tree's packages have now,
// under the build flags a stream's run recorded: the packages pkgs names.
func currentHashes(ctx context.Context, pkgs, build []string) (inputhash.Hashes, error) {
	if len(pkgs) == 0 {
		return inputhash.Hashes{}, nil
	}
	return inputhash.Current(ctx, "", pkgs, build, gotestrunner.StripNonLoadFlags(build))
}

// markFreshness judges each package of tree against the working tree:
// current, stale, or unknown without a hash.
func markFreshness(ctx context.Context, tree []*gotestspec.Package, events []gotestspec.TestEvent) error {
	var hashed []string
	for _, pkg := range tree {
		if pkg.InputHash != "" && !pkg.RunLevel {
			hashed = append(hashed, pkg.Path)
		}
	}
	current, err := currentHashes(ctx, hashed, streamBuild(events))
	if err != nil {
		return fmt.Errorf("--check-fresh: %w", err)
	}
	gotestspec.MarkFreshness(tree, current)
	return nil
}

// refuseWithoutInput reports, on stderr, a flag about a saved stream given
// without --input. Suppressing the verdict of a real run would turn a red
// pipeline green: --render-only makes sense only where the exit code
// describes a rendering. A run's own verdicts are current by construction,
// so --check-fresh has nothing to judge.
func refuseWithoutInput(own OwnFlags, input string) bool {
	switch {
	case input != "":
		return false
	case own.Bool("--render-only"):
		fmt.Fprintln(os.Stderr, "FAIL: --render-only requires --input")
	case own.Bool("--check-fresh"):
		fmt.Fprintln(os.Stderr, "FAIL: --check-fresh judges a saved stream against the working tree and requires --input")
	default:
		return false
	}
	return true
}
