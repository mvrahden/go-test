package gotestrunner

import (
	"bytes"
	"sync"
)

// liveSuite relays one suite process's -json events to the stream as its
// converter writes them, instead of when the process ends: each whole line
// at once, in the order written, under the collector's lock, read by a
// census of its own. Every line goes through what the burst at the process's
// end goes through, so only the time a line is written changes.
type liveSuite struct {
	c      *OutputCollector
	census *suiteCensus
	stamp  *stamper

	mu      sync.Mutex
	partial []byte
	// lines counts the whole lines relayed.
	lines int
}

// openLive is a relay for one suite process's events; nil when the run
// writes a suite's events when its process ends.
func (c *OutputCollector) openLive(test2json bool) *liveSuite {
	if !c.live || !test2json || c.mode != RunStreamJSON {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return &liveSuite{c: c, census: c.verdicts.suite(), stamp: newStamper()}
}

func (l *liveSuite) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeLocked(p)
	return len(p), nil
}

func (l *liveSuite) writeLocked(p []byte) {
	l.partial = append(l.partial, p...)
	for {
		i := bytes.IndexByte(l.partial, '\n')
		if i < 0 {
			break
		}
		l.relay(l.partial[:i+1])
		l.lines++
		l.partial = l.partial[i+1:]
	}
	l.partial = append([]byte(nil), l.partial...)
}

func (l *liveSuite) relay(line []byte) {
	l.c.mu.Lock()
	defer l.c.mu.Unlock()
	filterPackageLevelEvents(l.c.jsonWriterTo(l.census), l.stamp.line(line))
}

// redo takes the events a fresh converter wrote over the suite's whole
// output, after the first one ended early: the lines relayed already are its
// first ones, so only the rest go out. The first converter's cut last line
// is dropped, as its events are.
func (l *liveSuite) redo(events []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial = nil
	skip := l.lines
	for line := range bytes.Lines(events) {
		if skip > 0 && bytes.HasSuffix(line, []byte("\n")) {
			skip--
			continue
		}
		l.writeLocked(line)
	}
}

// flush relays a last line the converter left without its newline, once the
// converter has ended.
func (l *liveSuite) flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.partial) > 0 {
		l.relay(l.partial)
		l.partial = nil
	}
}
