package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/lint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func runLint(args []string, projectCfg config.ProjectConfig) int {
	if len(args) == 0 {
		args = []string{"./..."}
	}

	flagArgs := lintSkipFlags(args, projectCfg)
	os.Args = append([]string{"gotest-lint"}, append(flagArgs, args...)...)
	singlechecker.Main(lint.Analyzer)

	fmt.Fprintln(os.Stderr, "lint: unexpected return from singlechecker.Main")
	return 2
}

// lintSkipFlags returns analyzer flags derived from the config's lint.skip
// list, omitting any rules that are already set via CLI args.
func lintSkipFlags(args []string, cfg config.ProjectConfig) []string {
	var flags []string
	for _, rule := range cfg.Lint.Skip {
		flag := "-skip-" + rule
		if !slices.Contains(args, flag) && !slices.Contains(args, flag+"=true") && !slices.Contains(args, flag+"=false") {
			flags = append(flags, flag)
		}
	}
	return flags
}
