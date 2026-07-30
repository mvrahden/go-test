package testpkg

import (
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SetupHangTestSuite never returns from BeforeAll — a container start that
// wedged, a dial against a host that silently drops packets. Nothing can
// interrupt it, so the only thing the configured SetupTimeout can still buy is
// a verdict naming the budget that was blown, written while the setup is
// stuck rather than after it returns, because it never does.
type SetupHangTestSuite struct{}

func (s *SetupHangTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: 200 * time.Millisecond}
}

func (s *SetupHangTestSuite) BeforeAll(t *gotest.T) {
	fmt.Println("MARK:setup entered")
	select {}
}

func (s *SetupHangTestSuite) TestNeverReached(t *gotest.T) {
	fmt.Println("MARK:test ran")
}
