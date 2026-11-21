package vetone_test

import (
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// VetOneTestSuite fails vet with a finding of its own.
type VetOneTestSuite struct{}

func (s *VetOneTestSuite) TestPrints(t *gotest.T) { fmt.Printf("%d\n", "one") }
