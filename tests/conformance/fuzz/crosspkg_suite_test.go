package fuzz_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzCrossPackageTestSuite: an alias of another package's generic suite
// fuzzes with the methods and seeds that package declares.
type FuzzCrossPackageTestSuite struct{}

func (s *FuzzCrossPackageTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *FuzzCrossPackageTestSuite) TestGenericAlias(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fzcrosspkg/")
	gotest.NoError(t, err)
	t.When("a suite aliases a generic suite another package declares with a fuzz method", func(w *gotest.T) {
		w.It("the fuzz method's seeds replay and pass", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("FuzzIntCountTestSuite_FuzzCount/seed#0"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("a callback that takes the suite's type parameter fuzzes the type the alias instantiates", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("FuzzIntCountTestSuite_FuzzValue/seed#0"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
		w.It("a callback that takes a named type of the declaring package fuzzes that type", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("FuzzIntCountTestSuite_FuzzNamed/seed#0"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
}
