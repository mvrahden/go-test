package stdlibtests

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Plain stdlib tests living alongside a suite. They are counted separately from
// behaviors in the spec stats, and a run that mixes both must keep each
// visible instead of letting the suite tree swallow them.
type MixedTestSuite struct{}

func (s *MixedTestSuite) TestBehaviors(t *gotest.T) {
	t.When("a package mixes suites and stdlib tests", func(t *gotest.T) {
		t.It("still reports its behaviors", func(t *gotest.T) {
			gotest.Equal(t, 2, 1+1)
		})
	})
}

func TestPlainPasses(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("arithmetic is broken")
	}
}

func TestPlainSubtests(t *testing.T) {
	for _, name := range []string{"alpha", "beta"} {
		t.Run(name, func(t *testing.T) {
			if name == "" {
				t.Fatal("empty name")
			}
		})
	}
}
