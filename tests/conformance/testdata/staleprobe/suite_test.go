package staleprobe

import (
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"

	"staleprobe.example/staleprobe/dep"
)

// linked is set by -ldflags=-X.
var linked string

const testValue = 1

// StaleProbeTestSuite prints what its binary was built from: the
// dependency's value, the test file's, the build tag and the link flag.
type StaleProbeTestSuite struct{}

func (s *StaleProbeTestSuite) TestPrints(t *gotest.T) {
	fmt.Printf("stale-probe: dep=%d test=%d tag=%s linked=%s\n", dep.Value(), testValue, tag, linked)
}
