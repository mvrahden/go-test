//go:build foo

package staletags

import "github.com/mvrahden/go-test/pkg/gotest"

// TaggedTestSuite is built only with the foo tag.
type TaggedTestSuite struct{}

func (s *TaggedTestSuite) TestRuns(t *gotest.T) { gotest.True(t, true) }
