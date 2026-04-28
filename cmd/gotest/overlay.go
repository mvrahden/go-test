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

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	out := overlayOutput{
		OverlayFile: filepath.Join(tmpDir, "overlay.json"),
		Dir:         tmpDir,
	}
	data, err := json.Marshal(out)
	if err != nil {
		os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	fmt.Println(string(data))
	return 0
}
