package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/lint"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	// The singlechecker driver skips uncompilable packages and still exits 0,
	// so a broken tree lints "clean". Prove the targets compile first.
	if patterns := lint.PreflightPatterns(os.Args[1:]); len(patterns) > 0 {
		if err := lint.PreflightLoad("", patterns); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
			os.Exit(1)
		}
	}
	singlechecker.Main(lint.Analyzer)
}
