package control

import (
	"errors"
	"os"
	"time"
)

// Split feeds chunks to one line reader and returns the lines it parsed: the
// suite process's when up is set, else the runner's.
func Split(up bool, chunks ...string) []Line {
	r := lineReader{up: up}
	var out []Line
	for _, c := range chunks {
		out = r.feed([]byte(c), out)
	}
	return out
}

// Parse reads one line without its newline.
func Parse(s string) (Line, bool) { return parse([]byte(s)) }

// Format spells l as a line.
func Format(l Line) (string, error) {
	b, err := format(l)
	return string(b), err
}

// WriteRaw writes b to the suite process as it is.
func (l *Link) WriteRaw(b []byte) error {
	_, err := l.w.Write(b)
	return err
}

// WriteRaw writes b to the runner as it is.
func (e *Endpoint) WriteRaw(b []byte) error {
	_, err := e.w.Write(b)
	return err
}

// Handles are the descriptors or handles e reads from and writes to.
func (e *Endpoint) Handles() (r, w uintptr) {
	return handleOf(e.r), handleOf(e.w)
}

// handleOf reads f's descriptor without Fd, which on Windows would unbind it
// from the poller.
func handleOf(f *os.File) uintptr {
	var h uintptr
	if rc, err := f.SyscallConn(); err == nil {
		_ = rc.Control(func(fd uintptr) { h = fd })
	}
	return h
}

// Fill writes to the suite process until its end takes no more.
func (l *Link) Fill() error {
	chunk := make([]byte, 4096)
	for {
		if err := l.w.SetWriteDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			return err
		}
		_, err := l.w.Write(chunk)
		_ = l.w.SetWriteDeadline(time.Time{})
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
