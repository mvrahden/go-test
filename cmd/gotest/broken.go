package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

// reportBrokenPackages prints load diagnostics for broken packages in the
// `go build` shape and returns whether any were reported. Codegen commands
// (generate, prepare) fail fast on broken packages: generated output for a
// package that does not build is meaningless, and silence would leave stale
// artifacts in place.
func reportBrokenPackages(broken []gotestgen.BrokenPackage) bool {
	if len(broken) == 0 {
		return false
	}
	for i := range broken {
		fmt.Fprintf(os.Stderr, "# %s\n", broken[i].PkgPath)
		for _, e := range broken[i].Errors {
			fmt.Fprintln(os.Stderr, e)
		}
	}
	fmt.Fprintf(os.Stderr, "FAIL: %d package(s) failed to build\n", len(broken))
	return true
}
