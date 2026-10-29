package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DiscoveryAliasesTestSuite: an alias names the suite it stands for, at any
// depth; a defined type, a case with results and a shared fixture in a test
// file are rejected where they are written.
type DiscoveryAliasesTestSuite struct{}

func (s *DiscoveryAliasesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *DiscoveryAliasesTestSuite) TestAliasesAndSignatures(t *gotest.T) {
	t.When("a suite is declared through an alias or with an unusual signature", func(w *gotest.T) {
		w.It("a method declared on a pointer to an alias of the suite runs", func(it *gotest.T) {
			checkShape(it, shape{pkg: "aliasrecv", verdict: "TestRealTestSuite/TestViaAlias"})
		})
		w.It("an alias of an alias of a generic suite runs its methods", func(it *gotest.T) {
			checkShape(it, shape{pkg: "aliasalias", verdict: "TestIntTestSuite/TestZero"})
			checkShape(it, shape{pkg: "aliasalias", verdict: "TestIntAgainTestSuite/TestZero"})
		})
		w.It("a defined type over a suite is rejected at its declaration and pointed to an alias", func(it *gotest.T) {
			checkShape(it, shape{pkg: "sigdefined", exit: 2,
				shows: `sigdefined_suite_test\.go:10:6: suite CopyTestSuite is defined over suite ATestSuite and carries none of its methods; declare an alias \(= ATestSuite\) or a struct of its own`})
		})
		w.It("a defined type over a type that declares a case is rejected at its declaration and pointed to an alias", func(it *gotest.T) {
			checkShape(it, shape{pkg: "recvdefined", exit: 2,
				shows: `recvdefined_suite_test\.go:10:6: suite DefinedTestSuite is defined over cases and carries none of its methods; declare an alias \(= cases\) or a struct of its own`})
		})
		w.It("an alias runs the methods of the type it names, not those of a type named like the alias", func(it *gotest.T) {
			checkShape(it, shape{pkg: "recvcollide", verdict: "TestThingTestSuite/TestRemote"})
		})
		w.It("a Test method that returns a value is rejected at the method", func(it *gotest.T) {
			checkShape(it, shape{pkg: "sigresult", exit: 2,
				shows: `sigresult_suite_test\.go:9:22: TestBad on XTestSuite returns a value; a Test method returns nothing`})
		})
		w.It("a shared fixture declared in a _test.go file is rejected at its declaration", func(it *gotest.T) {
			checkShape(it, shape{pkg: "sharedintest", exit: 2,
				shows: `sharedintest_suite_test\.go:10:6: shared fixture PGSharedFixture is declared in a _test\.go file; the setup program cannot import it, move it to a non-test file`})
		})
	})
}
