package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CrossPackageTestSuite: a suite runs what another package declares for it,
// and what it binds from another package is held to the rules local
// declarations are held to.
type CrossPackageTestSuite struct{}

func (s *CrossPackageTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *CrossPackageTestSuite) TestAcrossPackages(t *gotest.T) {
	t.When("a suite reaches across packages", func(w *gotest.T) {
		w.It("an alias of a generic suite from another package runs that suite's methods", func(it *gotest.T) {
			checkShape(it, shape{pkg: "xpkgcontract", verdict: "TestIntIndexTestSuite/TestZeroValue"})
		})
		w.It("a package fixture from another package with a mis-signed hook is rejected with the signature it needs", func(it *gotest.T) {
			checkShape(it, shape{pkg: "xpkgfixture", exit: 2,
				shows: `fix\.go:11:\d+: package fixture "BadFixture": BeforeEach must have signature \(ctx context\.Context\) error`})
		})
	})
}
