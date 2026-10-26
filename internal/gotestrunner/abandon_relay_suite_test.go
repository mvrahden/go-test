package gotestrunner_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AbandonRelayTestSuite: a CLI giving up on its trees still passes on the
// events of the suites that ended, within its bound.
//
// Sequential: the relays are counted process-wide.
type AbandonRelayTestSuite struct{}

func (s *AbandonRelayTestSuite) TestAbandonWaitsForTheRelays(t *gotest.T) {
	t.When("a suite's events are yet to reach the stream", func(w *gotest.T) {
		w.It("returns once they did", func(it *gotest.T) {
			done := gotestrunner.ExportRelay()
			returned := make(chan struct{})
			gotest.Go(it, func() {
				gotestrunner.Abandon(time.Minute, nil)
				close(returned)
			})
			select {
			case <-returned:
				gotest.Fail(it, "Abandon returned before the events were passed on")
			case <-time.After(100 * time.Millisecond):
			}
			done()
			select {
			case <-returned:
			case <-time.After(5 * time.Second):
				gotest.Fail(it, "Abandon did not return once the events were passed on")
			}
		})
		w.It("returns at its bound when they never are", func(it *gotest.T) {
			done := gotestrunner.ExportRelay()
			began := time.Now()
			gotestrunner.Abandon(100*time.Millisecond, nil)
			elapsed := time.Since(began)
			done()
			gotest.Less(it, elapsed, 5*time.Second)
		})
		w.It("returns at once on a further signal", func(it *gotest.T) {
			done := gotestrunner.ExportRelay()
			hurry := make(chan struct{})
			close(hurry)
			began := time.Now()
			gotestrunner.Abandon(time.Minute, hurry)
			elapsed := time.Since(began)
			done()
			gotest.Less(it, elapsed, 5*time.Second)
		})
	})
}
