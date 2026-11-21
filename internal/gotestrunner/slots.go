package gotestrunner

import (
	"slices"
	"sync"
)

// slots are the suite processes a run may have at once, given out in the
// order suites queued for one: a suite queued earlier takes the next free
// slot before one queued later.
type slots struct {
	mu      sync.Mutex
	free    int
	waiting []chan struct{}
}

func newSlots(n int) *slots { return &slots{free: n} }

// queue enlists for a slot and returns what is closed once one is held.
func (s *slots) queue() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := make(chan struct{})
	if s.free > 0 && len(s.waiting) == 0 {
		s.free--
		close(held)
		return held
	}
	s.waiting = append(s.waiting, held)
	return held
}

// leave gives back the slot held names, or leaves the queue if it holds none
// yet.
func (s *slots) leave(held chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := slices.Index(s.waiting, held); i >= 0 {
		s.waiting = slices.Delete(s.waiting, i, i+1)
		return
	}
	if len(s.waiting) > 0 {
		close(s.waiting[0])
		s.waiting = s.waiting[1:]
		return
	}
	s.free++
}
