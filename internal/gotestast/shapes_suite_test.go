package gotestast_test

import (
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ShapesTestSuite covers which *TestSuite-named field types count as suites.
type ShapesTestSuite struct{}

func (s *ShapesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const shapesSrc = `package testpkg

type HelperTestSuite struct{}

func (s *HelperTestSuite) BeforeAll() {}
func (s *HelperTestSuite) Testify()   {}

type InnerTestSuite struct{}

func (s *InnerTestSuite) TestInner() {}

type HoldsHelperTestSuite struct {
	helper HelperTestSuite
	ptr    *HelperTestSuite
}

type HoldsInnerTestSuite struct {
	inner *InnerTestSuite
}
`

func (s *ShapesTestSuite) TestSuiteTypedFields(t *gotest.T) {
	fm := loadNamedSource(t.T(), "shapes.go", shapesSrc)

	t.When("a suite holds a *TestSuite-named type in a named field", func(w *gotest.T) {
		w.It("accepts one that declares no case: it is a helper type", func(it *gotest.T) {
			pos, err := gotestast.ValidateSuiteShape(suiteNamed(it, fm, "HoldsHelperTestSuite"))
			gotest.NoError(it, err)
			gotest.False(it, pos.IsValid())
		})
		w.It("rejects one that declares a case at the field", func(it *gotest.T) {
			pos, err := gotestast.ValidateSuiteShape(suiteNamed(it, fm, "HoldsInnerTestSuite"))
			gotest.Equal(it, "suite HoldsInnerTestSuite has a field of suite type InnerTestSuite; suites are never nested — share behavior through a generic contract suite", err.Error())
			gotest.Equal(it, 18, fm.pkg.Fset.Position(pos).Line)
		})
	})
}
