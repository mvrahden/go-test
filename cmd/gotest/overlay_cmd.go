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
	OverlayFile string `json:"overlayFile"`
	Dir         string `json:"dir"`
}

func runOverlay(args []string) int {
	patterns := ExtractPackagePatterns(args)

	var allResults gotestgen.GenerateResults

	for _, pattern := range patterns {
		results, err := gotestrunner.SuitesGenerate(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allResults = append(allResults, results...)
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
		OverlayFile: filepath.Join(tmpDir, "overlay.json"),
		Dir:         tmpDir,
	}

	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	return 0
}
