package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

// InputsTestSuite's per-input hooks get a real *testing.T under the fuzz
// wrapper, so their t.T() calls are fine.
type InputsTestSuite struct{}

func (s *InputsTestSuite) BeforeEach(t *gotest.T) { t.T().Log("input") }

func (s *InputsTestSuite) AfterEach(t *gotest.T) { t.T().Log("input done") }

func (s *InputsTestSuite) TestOne(t *gotest.T) {}

func (s *InputsTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}
