package staletags

import "github.com/mvrahden/go-test/pkg/gotest"

// UntaggedTestSuite is built with or without the tag.
type UntaggedTestSuite struct{}

func (s *UntaggedTestSuite) TestRuns(t *gotest.T) { gotest.True(t, true) }
