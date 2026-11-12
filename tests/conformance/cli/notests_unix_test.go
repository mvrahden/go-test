//go:build unix

package cli_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// The wrapper hands the binary a -test.run that matches none of its
// functions, as a binary built without the generated harness would run.
func (s *CLINoTestsTestSuite) TestASuiteWhoseBinaryRunsNoneOfItsTests(t *gotest.T) {
	wrap := filepath.Join(t.TempDir(), "wrap.sh")
	gotest.NoError(t, os.WriteFile(wrap, []byte("#!/bin/sh\nbin=\"$1\"; shift\nexec \"$bin\" \"$@\" -test.run='^NoSuchTest$'\n"), 0o700))
	run, err := testkit.RunCLI(t.Context(), ".", "-exec", wrap, "../testdata/passing/")
	gotest.NoError(t, err)
	t.When("a suite's binary runs none of its tests in a text run without a pattern", func(w *gotest.T) {
		w.It("fails the package and exits 2, naming the suite as one that ran no tests", func(it *gotest.T) {
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Regexp(it, `(?m)^FAIL\t\S+/testdata/passing\t`, run.Stdout)
			gotest.Contains(it, run.Stderr, "gotest: TestPassingTestSuite ran no tests")
		})
	})
}
