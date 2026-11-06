package control_test

import (
	"strconv"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// InheritanceTestSuite proves the channel stays with the suite process that
// claimed it: a process a test starts, directly or through a shell, neither
// holds it nor finds it named, so it can neither read a stop nor keep the
// channel open once the suite process is gone.
// Sequential: its methods start children and grandchildren and time the
// channel's end against a bound, which parallel children would stretch.
type InheritanceTestSuite struct{}

func (s *InheritanceTestSuite) BeforeEach(t *gotest.T) *children { return &children{} }

func (s *InheritanceTestSuite) AfterEach(t *gotest.T, cs *children) { cs.reap() }

func (s *InheritanceTestSuite) TestProcessesATestStarts(t *gotest.T, cs *children) {
	c := cs.start(t, spec{mode: "spawn", link: true, env: []string{envDir + "=" + t.TempDir()}})
	c.next(t)
	c.value(t, "done")
	for _, w := range spawnWays {
		if pid, err := strconv.Atoi(c.seen[w.name+".pid"]); err == nil {
			cs.probes = append(cs.probes, pid)
		}
	}
	code := c.wait(t)
	closed := c.closedWithin(testkit.Scaled(2 * time.Second))

	t.It("takes the variable out of the environment and the channel out of reach", func(it *gotest.T) {
		gotest.Equal(it, "false", c.seen["env"])
		checkClaimed(it, c)
		gotest.Zero(it, code)
	})
	for _, w := range spawnWays {
		t.It("keeps both from a process started by "+w.name, func(it *gotest.T) {
			gotest.Empty(it, c.seen[w.name+".error"])
			gotest.Equal(it, "false", c.seen[w.name+".env"])
			gotest.Equal(it, "none", c.seen[w.name+".held"])
		})
	}
	t.It("ends the channel with the suite process while those still run", func(it *gotest.T) {
		gotest.True(it, closed, "a started process holds the channel open")
	})
}

func (s *InheritanceTestSuite) TestClaim_Refused(t *gotest.T, cs *children) {
	notANumber, empty := "abc", ""
	entries := []struct {
		Desc      string
		spec      spec
		untouched bool
	}{
		{Desc: "no variable, as under plain go test", spec: spec{mode: "claim"}},
		{Desc: "a socket at descriptor 3 but no variable", spec: spec{mode: "claim", link: true, unnamed: true}},
		{Desc: "a variable naming a regular file", spec: spec{mode: "claim", file: true}, untouched: true},
		{Desc: "a variable that is no descriptor", spec: spec{mode: "claim", value: &notANumber}},
		{Desc: "an empty variable", spec: spec{mode: "claim", value: &empty}},
		{Desc: "a variable naming the standard streams", spec: spec{mode: "claimstd", socket: true, stdin: true}},
	}
	for t, e := range gotest.Each(t, entries) {
		c := cs.start(t, e.spec)
		claimed, env := c.value(t, "claimed"), c.value(t, "env")
		untouched := c.value(t, "untouched")
		code := c.wait(t)

		gotest.Equal(t, "false", claimed)
		gotest.Equal(t, "false", env)
		if e.untouched {
			gotest.Equal(t, "true", untouched, "the refused descriptor was changed")
		}
		gotest.Zero(t, code)
	}
}
