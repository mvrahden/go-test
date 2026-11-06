package control_test

import (
	"runtime"
	"strconv"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DescriptorLimitTestSuite proves a descriptor limit too low for the move
// leaves the channel where it is, still closed on exec.
type DescriptorLimitTestSuite struct{}

func (s *DescriptorLimitTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "Windows has no descriptor limit"
	}
	return ""
}

func (s *DescriptorLimitTestSuite) BeforeEach(t *gotest.T) *children { return &children{} }

func (s *DescriptorLimitTestSuite) AfterEach(t *gotest.T, cs *children) { cs.reap() }

func (s *DescriptorLimitTestSuite) TestLimitBelowTheMove(t *gotest.T, cs *children) {
	c := cs.start(t, spec{mode: "spawn", link: true, env: []string{envDir + "=" + t.TempDir(), envLimit + "=64"}})
	c.next(t)
	c.value(t, "done")
	for _, w := range spawnWays {
		if pid, err := strconv.Atoi(c.seen[w.name+".pid"]); err == nil {
			cs.probes = append(cs.probes, pid)
		}
	}
	code := c.wait(t)
	closed := c.closedWithin(testkit.Scaled(2 * time.Second))

	t.It("keeps the channel at its descriptor, closed on exec", func(it *gotest.T) {
		gotest.Equal(it, "3", c.seen["moved"])
		gotest.Equal(it, "true", c.seen["cloexec"])
		gotest.Zero(it, code)
	})
	for _, w := range spawnWays {
		t.It("keeps it from a process started by "+w.name, func(it *gotest.T) {
			gotest.Empty(it, c.seen[w.name+".error"])
			gotest.Equal(it, "none", c.seen[w.name+".held"])
		})
	}
	t.It("ends the channel with the suite process", func(it *gotest.T) {
		gotest.True(it, closed, "a started process holds the channel open")
	})
}
