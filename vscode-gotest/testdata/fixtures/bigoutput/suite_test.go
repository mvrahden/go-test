package bigoutput

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Output large enough to span several pipe chunks, including one line far
// longer than a typical 64KB read. The extension reassembles stdout into lines
// across chunk boundaries, so a single event split down the middle must still
// parse — this fixture is what makes that path run for real.
type BigOutputTestSuite struct{}

func (s *BigOutputTestSuite) TestVerbose(t *gotest.T) {
	t.When("a test emits a great deal of output", func(t *gotest.T) {
		t.It("survives a single very long line", func(t *gotest.T) {
			t.T().Log(strings.Repeat("x", 200_000))
			gotest.Equal(t, 1, 1)
		})
		t.It("survives many short lines", func(t *gotest.T) {
			for i := 0; i < 2_000; i++ {
				t.T().Log("line of diagnostic output number", i)
			}
			gotest.Equal(t, 1, 1)
		})
	})
}
