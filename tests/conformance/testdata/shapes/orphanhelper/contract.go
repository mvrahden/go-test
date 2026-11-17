package orphanhelper

import "github.com/mvrahden/go-test/pkg/gotest"

// Contract is declared in a non-test file: another package may call it.
type Contract struct{}

func (c *Contract) TestRoundTrip(t *gotest.T) {}
