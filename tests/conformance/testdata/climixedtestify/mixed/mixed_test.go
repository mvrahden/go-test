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

type LegacyTestSuite struct {
	suite.Suite
}

func (s *LegacyTestSuite) SetupSuite() {}

func (s LegacyTestSuite) TestValue() {}

func TestLegacy(t *testing.T) {
	t.Log("go test ran the testify runner")
	suite.Run(t, new(LegacyTestSuite))
}
