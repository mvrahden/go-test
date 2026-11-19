package vettwo_test

import (
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// VetTwoTestSuite fails vet with a finding of its own.
type VetTwoTestSuite struct{}

func (s *VetTwoTestSuite) TestPrints(t *gotest.T) { fmt.Printf("%d\n", "two") }
