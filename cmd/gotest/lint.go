package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/lint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func runLint(args []string) int {
	if len(args) == 0 {
		args = []string{"./..."}
	}

	os.Args = append([]string{"gotest-lint"}, args...)
	singlechecker.Main(lint.Analyzer)

	fmt.Fprintln(os.Stderr, "lint: unexpected return from singlechecker.Main")
	return 2
}
