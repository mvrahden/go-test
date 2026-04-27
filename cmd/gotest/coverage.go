package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/coverage"
)

func runCoverage(args []string) int {
	minPct, patterns := parseCoverageFlags(args)

	report, err := coverage.Analyze(patterns)
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
