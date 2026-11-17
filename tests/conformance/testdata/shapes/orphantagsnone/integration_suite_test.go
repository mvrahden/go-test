//go:build orphantagsint

package orphantagsnone

import "github.com/mvrahden/go-test/pkg/gotest"

type IntegrationTestSuite struct{}

func (s *IntegrationTestSuite) TestDB(t *gotest.T) { gotest.True(t, true) }
