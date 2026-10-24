package drill

import (
	"hash/fnv"
	"sync"
)

// Queues hands out a phase's mutants to its worker slots. Each mutant
// prefers the slot its name hashes to, whatever else is selected, so a
// rerun builds it in the same tree and finds it in the build cache. A slot
// whose own queue is empty takes from the fullest one.
type Queues struct {
	mu     sync.Mutex
	queues [][]string
}

// NewQueues spreads names, in order, over slots queues.
func NewQueues(names []string, slots int) *Queues {
	q := &Queues{queues: make([][]string, slots)}
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

// Next is slot's next mutant; false once every queue is empty.
func (q *Queues) Next(slot int) (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	from := slot
	if len(q.queues[slot]) == 0 {
		for i, queue := range q.queues {
			if len(queue) > len(q.queues[from]) {
				from = i
			}
		}
	}
	if len(q.queues[from]) == 0 {
		return "", false
	}
	name := q.queues[from][0]
	q.queues[from] = q.queues[from][1:]
	return name, true
}
