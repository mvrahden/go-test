package lintrules

import (
	"errors"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// harness has a T method of its own; it is no gotest.T.
type harness struct{ t *testing.T }

func (h *harness) T() *testing.T { return h.t }

func ready(t *gotest.T) { gotest.True(t, true) }

type RulesTestSuite struct{}

func (s *RulesTestSuite) TestOwnT(t *gotest.T) {
	h := &harness{t: t.T()}
	h.T().Setenv("LINTRULES", "1")
	_ = h.T().TempDir()
}

func (s *RulesTestSuite) TestPollHelper(t *gotest.T) {
	gotest.Eventually(t, time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		ready(t)
	})
}

func (s *RulesTestSuite) TestLenCheck(t *gotest.T) {
	items := []int{1}
	gotest.True(t, len(items) != 0)
}

func (s *RulesTestSuite) TestRedundant(t *gotest.T) {
	err := errors.New("boom")
	gotest.Error(t, err, "loading config") // the load must fail
	gotest.ErrorContains(t, err, "boom")
}

func (s *RulesTestSuite) TestSimplifyComment(t *gotest.T) {
	a, b := 1, 1
	gotest.True(t, a == /* same seed */ b, "seeded")
}
