package typos

import "github.com/mvrahden/go-test/pkg/gotest"

type TypoTestSuite struct{}

func (s *TypoTestSuite) BeforAll(t *gotest.T) {}

func (s *TypoTestSuite) X_AfterAll(t *gotest.T) {}

//nolint:lifecycle-typo // a helper, named on purpose
func (s *TypoTestSuite) AfterCall(t *gotest.T) {}

func (s *TypoTestSuite) TestSomething(t *gotest.T) {}

type Helpers struct{}

func (h *Helpers) BenchmarkLookup(b *gotest.B) {}

func (h *Helpers) FuzzParse(f *gotest.F) {}

func (h *Helpers) BenchmarkReport(name string) string { return name }
