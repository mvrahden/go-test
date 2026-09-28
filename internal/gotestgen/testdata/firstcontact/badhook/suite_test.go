package badhook

import "github.com/mvrahden/go-test/pkg/gotest"

type HookTestSuite struct{}

func (s *HookTestSuite) BeforeEach(t *gotest.T, extra int) {}

func (s *HookTestSuite) AfterEach(t *gotest.T, extra int, more int) {}

func (s *HookTestSuite) TestSomething(t *gotest.T) {}
