package gotest_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachFilterTestSuite compiles and runs a child test binary whose one test
// ranges gotest.Each over three entries, then drives it with the flags that
// used to deadlock it: testing.T.Run does not run the closure for a subtest
// filtered by -run/-skip, or once -failfast has tripped, and eachRun blocked
// forever on a handoff that was never coming — until -test.timeout shot the
// process with no AfterAll and no fixture teardown.
//
// The suite is sequential: each case owns the child process it spawns.
type EachFilterTestSuite struct{}

const eachChildTimeout = 60 * time.Second
const eachChildWallClock = 120 * time.Second

const eachChildSource = `package eachchild

import (
	"fmt"
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestEach(t *testing.T) {
	tt := gotest.NewT(t)
	for sub, n := range gotest.Each(tt, []int{10, 20, 30}) {
		fmt.Fprintf(os.Stdout, "MARK:ran %d\n", n)
		if os.Getenv("GOTEST_TEST_EACH_FAIL_FIRST") != "" && n == 10 {
			sub.T().Fail()
		}
	}
}
`

// buildEachChild materializes a throwaway module around eachChildSource in
// dir and compiles its test binary into binDir, following the same
// replace-directive recipe the generator's own child harness uses.
func buildEachChild(t *gotest.T, dir, binDir string) string {
	modRoot, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)

	goMod := "module eachchild\n\ngo 1.25\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => " + modRoot + "\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "each_test.go"), []byte(eachChildSource), 0o600))

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "GOWORK=off")
	out, err := tidy.CombinedOutput()
	gotest.NoError(t, err, "go mod tidy: %s", out)

	bin := filepath.Join(binDir, "each.test")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "test", "-c", "-o", bin, ".") //nolint:gosec // G204: go tool with test-controlled arguments
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	out, err = build.CombinedOutput()
	gotest.NoError(t, err, "compiling the Each child failed:\n%s", out)
	return bin
}

// runEachChild executes the child with the given flags and env, bounded well
// below the assertion threshold so a regression fails by verdict, not by
// stalling this package.
func runEachChild(bin string, env []string, args ...string) (output string, elapsed time.Duration, passed bool) {
	ctx, cancel := context.WithTimeout(context.Background(), eachChildWallClock)
	args = append(args, "-test.v", "-test.timeout="+eachChildTimeout.String())
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // G204: freshly compiled test binary
	cmd.Env = append(os.Environ(), env...)

	start := time.Now()
	out, err := cmd.CombinedOutput()
	cancel()
	return string(out), time.Since(start), err == nil
}

func (s *EachFilterTestSuite) TestFilteredEntry(t *gotest.T) {
	bin := buildEachChild(t, t.TempDir(), t.TempDir())

	t.When("a single table entry is selected with -run", func(w *gotest.T) {
		output, elapsed, passed := runEachChild(bin, nil, "-test.run", "TestEach/#1")

		w.It("completes instead of hanging until the timeout kills it", func(it *gotest.T) {
			gotest.NotContains(it, output, "test timed out",
				"eachRun blocked on a subtest handoff that -run filtered away:\n%s", output)
			gotest.Less(it, elapsed, eachChildTimeout,
				"child ran %s, at or beyond its own -test.timeout:\n%s", elapsed, output)
			gotest.True(it, passed, "the selected entry passes, so the child must too:\n%s", output)
		})

		w.It("runs exactly the selected entry", func(it *gotest.T) {
			gotest.NotContains(it, output, "MARK:ran 10", "entry #0 was filtered out:\n%s", output)
			gotest.Contains(it, output, "MARK:ran 20", "entry #1 was selected:\n%s", output)
			gotest.NotContains(it, output, "MARK:ran 30", "entry #2 was filtered out:\n%s", output)
		})
	})
}

func (s *EachFilterTestSuite) TestFailFast(t *gotest.T) {
	bin := buildEachChild(t, t.TempDir(), t.TempDir())

	t.When("-failfast trips on the first entry", func(w *gotest.T) {
		output, elapsed, passed := runEachChild(bin,
			[]string{"GOTEST_TEST_EACH_FAIL_FIRST=1"}, "-test.failfast")

		w.It("finishes red instead of hanging on the suppressed entries", func(it *gotest.T) {
			gotest.NotContains(it, output, "test timed out",
				"eachRun blocked on a subtest failfast suppressed:\n%s", output)
			gotest.Less(it, elapsed, eachChildTimeout,
				"child ran %s, at or beyond its own -test.timeout:\n%s", elapsed, output)
			gotest.False(it, passed, "the first entry failed; the child must fail:\n%s", output)
			gotest.Contains(it, output, "MARK:ran 10",
				"the first entry runs before failfast trips:\n%s", output)
		})
	})
}
