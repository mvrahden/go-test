package withalias

import gt "github.com/mvrahden/go-test/pkg/gotest"

// The signature check reads the parameter's type, not the import's name:
// an aliased gotest import declares the same *gotest.T.
type AliasTestSuite struct{}

func (s *AliasTestSuite) TestAliased(t *gt.T) { gt.True(t, true) }
