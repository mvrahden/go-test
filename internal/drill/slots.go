package drill

import (
	"hash/fnv"
	"sync"
)

// Queues hands out a phase's mutants to its worker slots. Each mutant
// prefers the slot its name hashes to, whatever else is selected, so a
// rerun builds it in the same tree and finds it in the build cache. A slot
// whose own queue is empty takes the last mutant of the busy slot with the
// most expected seconds left; a slot that has not asked yet is not robbed.
type Queues struct {
	mu     sync.Mutex
	asked  *sync.Cond
	queues [][]string
	weight func(string) float64
	state  []slotState
}

type slotState int

const (
	unasked slotState = iota // has not asked yet, or is asking
	busy                     // judging a mutant
	done                     // asks for no more
)

// NewQueues spreads names, in order, over slots queues; weight is a
// mutant's expected seconds, nil weighs all alike.
func NewQueues(names []string, slots int, weight func(string) float64) *Queues {
	if weight == nil {
		weight = func(string) float64 { return 1 }
	}
	q := &Queues{queues: make([][]string, slots), weight: weight, state: make([]slotState, slots)}
	q.asked = sync.NewCond(&q.mu)
	for _, name := range names {
		i := Slot(name, slots)
		q.queues[i] = append(q.queues[i], name)
	}
	return q
}

// Slot is the slot, of slots, that name prefers.
func Slot(name string, slots int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int(h.Sum32() % uint32(slots)) //nolint:gosec // G115: slots is a small positive count
}

// Next is slot's next mutant; false once every queue is empty. It waits
// while only slots that have not asked yet have mutants left.
func (q *Queues) Next(slot int) (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	defer q.asked.Broadcast()
	q.state[slot] = unasked
	for {
		if own := q.queues[slot]; len(own) > 0 {
			q.queues[slot], q.state[slot] = own[1:], busy
			return own[0], true
		}
		from, most, left := -1, 0.0, false
		for i, queue := range q.queues {
			if len(queue) == 0 {
				continue
			}
			left = true
			if q.state[i] == unasked {
				continue
			}
			if w := q.remaining(queue); from < 0 || w > most {
				from, most = i, w
			}
		}
		if from >= 0 {
			queue := q.queues[from]
			q.queues[from], q.state[slot] = queue[:len(queue)-1], busy
			return queue[len(queue)-1], true
		}
		if !left {
			return "", false
		}
		q.asked.Wait()
	}
}

// Done says slot asks for no more; what is left in its queue goes to the
// others.
func (q *Queues) Done(slot int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.state[slot] = done
	q.asked.Broadcast()
}

func (q *Queues) remaining(queue []string) float64 {
	var sum float64
	for _, name := range queue {
		sum += q.weight(name)
	}
	return sum
}
