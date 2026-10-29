package discovery_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CrossPackageTestSuite: a suite runs what another package declares for it,
// the external test package included, and what it binds from another package
// is held to the rules local declarations are held to.
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
		w.It("a package fixture from another package with a mis-signed FixtureConfig is rejected at the method", func(it *gotest.T) {
			checkShape(it, shape{pkg: "xpkgconfig", exit: 2,
				shows: `fix\.go:11:\d+: unsupported return type for "ConfiguredFixture\.FixtureConfig": expected gotest\.FixtureConfig, got int`})
		})
		w.It("a package fixture from another package with Hydrate is rejected at the method", func(it *gotest.T) {
			checkShape(it, shape{pkg: "xpkghydrate", exit: 2,
				shows: `fix\.go:11:\d+: package fixture "HydratedFixture" must not have Hydrate method; Hydrate/Dehydrate are for shared fixtures only`})
		})
	})
}

func (s *CrossPackageTestSuite) TestGenericAliasesInTheExternalTestPackage(t *gotest.T) {
	t.When("the external test package declares an alias of a generic suite", func(w *gotest.T) {
		w.It("an alias of a generic suite the package under test declares runs that suite's methods", func(it *gotest.T) {
			checkShape(it, shape{pkg: "pxgenericalias", verdict: "TestIntContractTestSuite/TestRuns"})
		})
		w.It("an alias of a generic suite the external test package declares runs that suite's methods", func(it *gotest.T) {
			checkShape(it, shape{pkg: "pxgenericalias", verdict: "TestIntLocalTestSuite/TestLocal"})
		})
		w.It("an alias of a generic suite from a third package runs that suite's methods", func(it *gotest.T) {
			checkShape(it, shape{pkg: "pxgenericalias", verdict: "TestIntIndexTestSuite/TestZeroValue"})
		})
	})
}
