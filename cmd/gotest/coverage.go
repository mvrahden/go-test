package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/coverage"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

func runCoverage(args []string) int {
	minPct, patterns := parseCoverageFlags(args)

	// 1. Generate test overlay
	var allResults gotestgen.GenerateResults
	for _, pattern := range patterns {
		results, _, err := gotestrunner.SuitesGenerateWithCollectorResults(pattern)
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
	defer os.RemoveAll(tmpDir)

	overlayPath := filepath.Join(tmpDir, "overlay.json")

	// 2. Run tests with coverage profiling
	coverFile, err := os.CreateTemp("", "gotest-cover-*.out")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	coverPath := coverFile.Name()
	coverFile.Close()
	defer os.Remove(coverPath)

	goTestArgs := []string{"test",
		"-overlay=" + overlayPath,
		"-coverprofile=" + coverPath,
	}
	goTestArgs = append(goTestArgs, patterns...)

	cmd := exec.Command("go", goTestArgs...)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	cmd.Run() // profile may still be usable even if some tests fail

	info, err := os.Stat(coverPath)
	if err != nil || info.Size() == 0 {
		fmt.Fprintf(os.Stderr, "FAIL: no coverage data collected (tests may have failed to compile)\n")
		return 2
	}

	// 3. Analyze coverage
	report, err := coverage.Analyze(coverPath, patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	coverage.Render(os.Stdout, report)

	if minPct > 0 && report.Total > 0 {
		pct := report.Covered * 100 / report.Total
		if pct < minPct {
			fmt.Fprintf(os.Stderr, "\nFAIL: %d%% semantic coverage (minimum %d%%)\n", pct, minPct)
			return 1
		}
	}

	return 0
}

func parseCoverageFlags(args []string) (minPct int, patterns []string) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--min" && i+1 < len(args):
			minPct, _ = strconv.Atoi(args[i+1])
			i++
		case strings.HasPrefix(args[i], "--min="):
			minPct, _ = strconv.Atoi(strings.TrimPrefix(args[i], "--min="))
		default:
			patterns = append(patterns, args[i])
		}
	}
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	return
}
