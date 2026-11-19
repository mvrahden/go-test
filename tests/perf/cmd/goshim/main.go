// Command goshim stands in for go: it appends its arguments, joined by \x1f,
// as one line to the file GOTEST_PERF_GOLOG names, then runs the go command
// GOTEST_PERF_REAL_GO names with the same arguments and streams.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mvrahden/go-test/tests/perf/internal/passthrough"
)

func main() {
	if err := passthrough.Log(os.Getenv("GOTEST_PERF_GOLOG"),strings.Join(os.Args[1:], "\x1f")+"\n"); err != nil {
		fmt.Fprintln(os.Stderr, "goshim:", err)
		os.Exit(2)
	}
	os.Exit(passthrough.Run(os.Getenv("GOTEST_PERF_REAL_GO"), os.Args[1:]))
}
