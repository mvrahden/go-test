package specslashes_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/specslashes"
)

// TwinTestSuite is declared again in the internal test package.
type TwinTestSuite struct{}

func (s *TwinTestSuite) TestShared(t *gotest.T) {
	t.When("the twin is external", func(w *gotest.T) {
		w.When("it nests", func(w *gotest.T) {
			w.It("reads the exported API", func(it *gotest.T) {
				gotest.Equal(it, "specslashes", specslashes.Name())
			})
		})
	})
}

func (s *TwinTestSuite) TestExternalOnly(t *gotest.T) {
	gotest.NotEmpty(t, specslashes.Name())
}

// LoneTestSuite has no twin in the internal test package.
type LoneTestSuite struct{}

func (s *LoneTestSuite) TestAlone(t *gotest.T) {
	gotest.NotEmpty(t, specslashes.Name())
}
