package fuzzcrash

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MessageTestSuite struct{}

// FuzzOnlySeed fails on every input but its seed, so a real session finds a
// crasher within its first mutations.
func (s *MessageTestSuite) FuzzOnlySeed(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {
		if in != "seed" {
			t.Errorf("unexpected input %q", in)
		}
	})
}

// FuzzTrim holds: a planted crasher for it reads as "no longer failing".
func (s *MessageTestSuite) FuzzTrim(f *gotest.F) {
	f.Add("  x ")
	f.Fuzz(func(t *gotest.T, in string) {
		trimmed := strings.TrimSpace(in)
		gotest.Equal(t, trimmed, strings.TrimSpace(trimmed))
	})
}

// FuzzSummary is the struct-typed target: Message fans out per field, so a
// crasher file holds one native value per leaf and triage shows the literal.
func (s *MessageTestSuite) FuzzSummary(f *gotest.F) {
	f.Add(Message{To: "a@b.c", Subject: "welcome", Priority: PriorityHigh})
	f.Fuzz(func(t *gotest.T, m Message) {
		out := Summary(m)
		gotest.Contains(t, out, m.Subject)
		gotest.Contains(t, out, m.To)
		gotest.Regexp(t, `^\[(LOW|NORMAL|HIGH)\] `, out)
	})
}
