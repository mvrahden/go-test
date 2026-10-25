// Command render prints all code gotest generates for the packages its
// arguments match, the shared fixture program included, which a run builds
// and deletes.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

func main() {
	loaded, broken, _, err := gotestgen.LoadPackagesMatching(os.Args[1:], nil)
	if err != nil {
		fail(err)
	}
	if len(broken) > 0 {
		fail(fmt.Errorf("%s: %s", broken[0].PkgPath, strings.Join(broken[0].Errors, "\n")))
	}
	results, shared, err := gotestgen.GenerateFromLoaded(loaded)
	if err != nil {
		fail(err)
	}
	for _, r := range results {
		os.Stdout.Write(r.PTest)
		os.Stdout.Write(r.PXTest)
	}
	if len(shared) > 0 {
		setup, err := gotestgen.GenerateSharedSetup(shared)
		if err != nil {
			fail(err)
		}
		os.Stdout.Write(setup)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "render:", err)
	os.Exit(1)
}
