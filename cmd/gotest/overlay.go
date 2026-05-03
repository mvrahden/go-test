package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

type overlayOutput struct {
	OverlayFile    string                       `json:"overlayFile"`
	Dir            string                       `json:"dir"`
	SharedFixtures []gotestgen.SharedFixtureInfo `json:"sharedFixtures,omitempty"`
}

func runOverlay(args []string) int {
	patterns := ExtractPackagePatterns(args)

	var allResults gotestgen.GenerateResults
	sharedSeen := map[string]bool{}
	var allSharedFixtures []gotestgen.SharedFixtureInfo
	for _, pattern := range patterns {
		results, sharedFixtures, err := gotestgen.GenerateWithSharedFixtures(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allResults = append(allResults, results...)
		for _, sf := range sharedFixtures {
			key := sf.PkgPath + "." + sf.Identifier
			if !sharedSeen[key] {
				sharedSeen[key] = true
				allSharedFixtures = append(allSharedFixtures, sf)
			}
		}
	}

	if len(allResults) == 0 {
		fmt.Fprintf(os.Stderr, "no suites found\n")
		return 1
	}

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	out := overlayOutput{
		OverlayFile:    filepath.Join(tmpDir, "overlay.json"),
		Dir:            tmpDir,
		SharedFixtures: allSharedFixtures,
	}

	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	return 0
}
