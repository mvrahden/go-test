package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/lint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func runLint(inv Invocation) int { //nolint:gocritic
	args := inv.Args
	if len(args) == 0 {
		args = []string{"./..."}
	}

	flagArgs, err := lintSkipFlags(args, inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	os.Args = append([]string{"gotest-lint"}, append(flagArgs, args...)...)
	singlechecker.Main(lint.Analyzer)

	fmt.Fprintln(os.Stderr, "lint: unexpected return from singlechecker.Main")
	return 2
}

// lintSkipFlags returns analyzer flags derived from the config's lint.skip
// list, omitting any rules that are already set via CLI args.
func lintSkipFlags(args []string, cfg config.ProjectConfig) ([]string, error) { //nolint:gocritic
	var flags []string
	for _, rule := range cfg.Lint.Skip {
		if !lint.SkippableRules[lint.Rule(rule)] {
			return nil, fmt.Errorf("unknown lint rule in %s: %q", config.FileName, rule)
		}
		flag := "-skip-" + rule
		if !slices.Contains(args, flag) && !slices.Contains(args, flag+"=true") && !slices.Contains(args, flag+"=false") {
			flags = append(flags, flag)
		}
	}
	return flags, nil
}
