package gotestruntime_test

import (
	"os"
	"os/exec"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// childEnv marks a child of the test binary, which runs a scenario whose
// verdict is meant to fail.
const childEnv = "GOTESTRUNTIME_CHILD"

// inChild reports whether this process is such a child.
func inChild() bool { return os.Getenv(childEnv) != "" }

// runChild runs suite's method in a child of the test binary and returns
// its output; the child's own verdict is the scenario's, not this test's.
// go test's own timeout ends a child whose scenario hangs.
func runChild(t *gotest.T, suite, method string) string {
	child := exec.CommandContext(t.Context(), os.Args[0], //nolint:gosec // G204: test-only subprocess
		"-test.run=^"+suite+"$/^"+method+"$", "-test.v", "-test.count=1", "-test.timeout=20s")
	child.Env = append(os.Environ(), childEnv+"=1")
	out, _ := child.CombinedOutput()
	return string(out)
}
