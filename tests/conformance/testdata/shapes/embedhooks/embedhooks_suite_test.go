package embedhooks

import "github.com/mvrahden/go-test/pkg/gotest"

// base is a plain type whose method set carries a name gotest reads.
type base struct{}

func (b *base) TestFromBase(t *gotest.T) {}

// HookedTestSuite embeds it, so TestFromBase is promoted but never a method of its own.
type HookedTestSuite struct {
	base
}

func (s *HookedTestSuite) TestOwn(t *gotest.T) {}
