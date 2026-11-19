package vetfail_test

import (
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// VetFailTestSuite compiles but fails vet: a build of it fails.
type VetFailTestSuite struct{}

func (s *VetFailTestSuite) TestPrints(t *gotest.T) { fmt.Printf("%d\n", "not a number") }
