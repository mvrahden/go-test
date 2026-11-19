// Command execcount is a go test -exec program: it appends one line to the
// file GOTEST_PERF_EXECLOG names, then runs os.Args[1] with os.Args[2:].
package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/tests/perf/internal/passthrough"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: execcount <program> [args...]")
		os.Exit(2)
	}
	if err := passthrough.Log(os.Getenv("GOTEST_PERF_EXECLOG"), "exec\n"); err != nil {
		fmt.Fprintln(os.Stderr, "execcount:", err)
		os.Exit(2)
	}
	os.Exit(passthrough.Run(os.Args[1], os.Args[2:]))
}
