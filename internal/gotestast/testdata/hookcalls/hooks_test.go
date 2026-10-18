package hookcalls

import "github.com/mvrahden/go-test/pkg/gotest"

// SeededTestSuite has fuzz methods and three hooks that call t.T().
type SeededTestSuite struct{}

func (s *SeededTestSuite) AfterEach(t *gotest.T) { _ = t.T() }

func (s *SeededTestSuite) BeforeAll(t *gotest.T) { _ = t.T() }

func (s *SeededTestSuite) BeforeEach(t *gotest.T) {}

func (s *SeededTestSuite) AfterAll(t *gotest.T) { _ = t.T() }

func (s *SeededTestSuite) FuzzSeed(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, s string) {})
}
