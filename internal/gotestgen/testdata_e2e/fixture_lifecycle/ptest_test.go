package fixturepkg

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type AppTestSuite struct {
	App *AppFixture
}

func (s *AppTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *AppTestSuite) BeforeAll(t *gotest.T)  {}
func (s *AppTestSuite) AfterAll(t *gotest.T)   {}
func (s *AppTestSuite) BeforeEach(t *gotest.T) {}
func (s *AppTestSuite) AfterEach(t *gotest.T)  {}

func (s *AppTestSuite) TestCreate(t *gotest.T) { DoWork() }
func (s *AppTestSuite) TestDelete(t *gotest.T) { DoWork() }
