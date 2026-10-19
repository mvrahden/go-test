// Package fzpromote is the package the promote clauses stage outside the
// repository: one target that fails on every input but its seed, one that
// holds, and one that decodes a struct from several leaves.
package fzpromote

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PromoteTestSuite struct{}

// FuzzOnlySeed fails on every input but its seed, so a session finds a
// crasher within its first mutations.
func (s *PromoteTestSuite) FuzzOnlySeed(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {
		if in != "seed" {
			t.Errorf("unexpected input %q", in)
		}
	})
}

// FuzzTrim holds on every input.
func (s *PromoteTestSuite) FuzzTrim(f *gotest.F) {
	f.Add(" x ")
	f.Fuzz(func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// Message fans out to one leaf per field.
type Message struct {
	To      string
	Subject string
	Count   int
}

// FuzzMessage decodes a Message from three leaves.
func (s *PromoteTestSuite) FuzzMessage(f *gotest.F) {
	f.Add(Message{To: "a@b.c", Subject: "hi", Count: 1})
	f.Fuzz(func(t *gotest.T, m Message) {
		gotest.GreaterOrEqual(t, len(m.To+m.Subject), 0)
	})
}
