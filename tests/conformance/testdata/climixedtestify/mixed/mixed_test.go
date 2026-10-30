package mixed

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MigratedTestSuite struct{}

func (s *MigratedTestSuite) TestMigrated(t *gotest.T) {
	gotest.True(t, true)
}

// BaseTestSuite declares no case: a testify base, left to testify.
type BaseTestSuite struct {
	suite.Suite
}

func (s *BaseTestSuite) SetupSuite() {}

type LegacyTestSuite struct {
	BaseTestSuite
}

func (s LegacyTestSuite) TestValue() {}

func TestLegacy(t *testing.T) {
	t.Log("go test ran the testify runner")
	suite.Run(t, new(LegacyTestSuite))
}
