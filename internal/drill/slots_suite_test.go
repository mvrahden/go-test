package drill_test

import (
	"strconv"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/drill"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SlotsTestSuite: a mutant is judged on the slot its name prefers, so a
// rerun builds it in the same tree; a slot with nothing of its own helps a
// busy one.
type SlotsTestSuite struct{}

func (s *SlotsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// own is names whose preferred slot is slot, in order.
func own(names []string, slot, slots int) []string {
	var mine []string
	for _, name := range names {
		if drill.Slot(name, slots) == slot {
			mine = append(mine, name)
		}
	}
	return mine
}

// preferring is n names that prefer slot, of slots.
func preferring(slot, slots, n int) []string {
	var names []string
	for i := 0; len(names) < n; i++ {
		if name := "m" + strconv.Itoa(i); drill.Slot(name, slots) == slot {
			names = append(names, name)
		}
	}
	return names
}

// drain runs one goroutine per slot until the queues are empty, each
// holding every mutant it takes for hold, and returns which slot judged
// what. Slots in skip ask for nothing.
func drain(t *gotest.T, q *drill.Queues, slots int, hold time.Duration, skip ...int) map[string][]int {
	var mu sync.Mutex
	judged := map[string][]int{}
	var wg sync.WaitGroup
	for slot := range slots {
		wg.Go(func() {
			defer q.Done(slot)
			for _, s := range skip {
				if s == slot {
					return
				}
			}
			for {
				name, ok := q.Next(slot)
				if !ok {
					return
				}
				mu.Lock()
				judged[name] = append(judged[name], slot)
				mu.Unlock()
				time.Sleep(hold)
			}
		})
	}
	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		gotest.Fail(t, "a slot is still waiting for a mutant")
	}
	return judged
}

func (s *SlotsTestSuite) TestAMutantKeepsItsSlotWhateverElseIsSelected(t *gotest.T) {
	all := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	for _, names := range [][]string{all, {"c", "h", "a"}, {"j", "e", "a", "b"}} {
		for slot := range 3 {
			mine := own(names, slot, 3)
			q := drill.NewQueues(names, 3, nil)
			var got []string
			for range mine {
				name, _ := q.Next(slot)
				got = append(got, name)
			}
			gotest.Equal(t, mine, got, "slot %d among %v", slot, names)
		}
	}
	gotest.NotEqual(t, own(all, 0, 3), all, "the names spread over the slots")
}

func (s *SlotsTestSuite) TestFewerMutantsThanSlotsRunOnTheirOwnSlots(t *gotest.T) {
	const slots = 4
	var names []string
	for slot := range slots - 1 {
		names = append(names, preferring(slot, slots, 1)...)
		for trial := range 100 {
			judged := drain(t, drill.NewQueues(names, slots, nil), slots, 0)
			for _, name := range names {
				gotest.Equal(t, []int{drill.Slot(name, slots)}, judged[name], "%s among %v, trial %d", name, names, trial)
			}
		}
	}
}

func (s *SlotsTestSuite) TestAnIdleSlotTakesTheLastMutantOfABusyOne(t *gotest.T) {
	mine := preferring(0, 3, 5)
	q := drill.NewQueues(mine, 3, nil)
	first, ok := q.Next(0)
	gotest.True(t, ok)
	gotest.Equal(t, mine[0], first, "the owner takes its next")
	var stolen []string
	for _, slot := range []int{1, 2, 1, 2} {
		name, ok := q.Next(slot)
		gotest.True(t, ok)
		stolen = append(stolen, name)
	}
	gotest.Equal(t, []string{mine[4], mine[3], mine[2], mine[1]}, stolen, "the others take from its end")
	_, ok = q.Next(0)
	gotest.False(t, ok)
}

func (s *SlotsTestSuite) TestAnIdleSlotTakesFromTheHeaviestQueue(t *gotest.T) {
	light, heavy := preferring(0, 3, 3), preferring(1, 3, 2)
	weights := map[string]float64{}
	for _, name := range light {
		weights[name] = 1
	}
	for _, name := range heavy {
		weights[name] = 10
	}
	q := drill.NewQueues(append(light, heavy...), 3, func(name string) float64 { return weights[name] })
	_, _ = q.Next(0)
	_, _ = q.Next(1)
	name, ok := q.Next(2)
	gotest.True(t, ok)
	gotest.Equal(t, heavy[1], name, "one heavy mutant outweighs two light ones")
}

func (s *SlotsTestSuite) TestEveryMutantIsJudgedOnceAndNoSlotStarves(t *gotest.T) {
	var names []string
	for i := range 40 {
		names = append(names, "n"+strconv.Itoa(i))
	}
	judged := drain(t, drill.NewQueues(names, 4, nil), 4, time.Millisecond)
	gotest.Len(t, judged, len(names))
	for _, name := range names {
		gotest.Len(t, judged[name], 1, "%s judged once", name)
	}

	idle := preferring(1, 3, 4)
	judged = drain(t, drill.NewQueues(idle, 3, nil), 3, time.Millisecond, 1)
	gotest.Len(t, judged, len(idle), "a slot that asks for nothing leaves its mutants to the others")
}
