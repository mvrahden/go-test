package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SlotsTestSuite: the suite slots of a run go to suites in the order they
// queued.
type SlotsTestSuite struct{}

func held(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

func (s *SlotsTestSuite) TestOrder(t *gotest.T) {
	t.When("one slot is held and three suites queue behind it", func(w *gotest.T) {
		sl := gotestrunner.ExportNewSlots(1)
		first := gotestrunner.ExportQueue(sl)
		queued := []chan struct{}{gotestrunner.ExportQueue(sl), gotestrunner.ExportQueue(sl), gotestrunner.ExportQueue(sl)}
		w.It("the first holds the slot and the others wait", func(it *gotest.T) {
			gotest.True(it, held(first))
			gotest.False(it, held(queued[0]) || held(queued[1]) || held(queued[2]))
		})
		w.It("each slot given back goes to the suite that queued next", func(it *gotest.T) {
			gotestrunner.ExportLeave(sl, first)
			gotest.True(it, held(queued[0]))
			gotest.False(it, held(queued[1]))
			gotestrunner.ExportLeave(sl, queued[0])
			gotest.True(it, held(queued[1]))
			gotest.False(it, held(queued[2]))
		})
	})
	t.When("a suite leaves the queue before it holds a slot", func(w *gotest.T) {
		sl := gotestrunner.ExportNewSlots(1)
		first := gotestrunner.ExportQueue(sl)
		gone := gotestrunner.ExportQueue(sl)
		next := gotestrunner.ExportQueue(sl)
		gotestrunner.ExportLeave(sl, gone)
		gotestrunner.ExportLeave(sl, first)
		w.It("the slot goes to the one behind it", func(it *gotest.T) {
			gotest.False(it, held(gone))
			gotest.True(it, held(next))
		})
		w.It("a slot given back with no one queued is taken at once", func(it *gotest.T) {
			gotestrunner.ExportLeave(sl, next)
			gotest.True(it, held(gotestrunner.ExportQueue(sl)))
		})
	})
}
