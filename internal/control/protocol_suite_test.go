package control_test

import (
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ProtocolTestSuite covers the lines: what parses to what, what is dropped,
// and what a side refuses to send.
type ProtocolTestSuite struct{}

func (s *ProtocolTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ProtocolTestSuite) TestParse(t *gotest.T) {
	entries := []struct {
		Desc string
		line string
		want control.Line
	}{
		{Desc: "a hello", line: "hello 1 4242", want: control.Line{Kind: control.Hello, Version: 1, PID: 4242}},
		{Desc: "a hello of a later version", line: "hello 2 7", want: control.Line{Kind: control.Hello, Version: 2, PID: 7}},
		{Desc: "a budget", line: "budget 5m30s", want: control.Line{Kind: control.Budget, Duration: 5*time.Minute + 30*time.Second}},
		{Desc: "a zero budget", line: "budget 0s", want: control.Line{Kind: control.Budget}},
		{Desc: "a stop by a signal", line: "stopped signal:SIGTERM", want: control.Line{Kind: control.Stopped, Reason: "signal:SIGTERM"}},
		{Desc: "a stop by a runner gone", line: "stopped runner-gone", want: control.Line{Kind: control.Stopped, Reason: "runner-gone"}},
		{Desc: "a stop", line: "stop", want: control.Line{Kind: control.Stop}},
		{Desc: "a force", line: "force", want: control.Line{Kind: control.Force}},
	}
	for t, e := range gotest.Each(t, entries) {
		got, ok := control.Parse(e.line)
		gotest.True(t, ok, "%q did not parse", e.line)
		gotest.Equal(t, e.want, got)
	}
}

func (s *ProtocolTestSuite) TestParse_Malformed(t *gotest.T) {
	entries := []struct {
		Desc string
		line string
	}{
		{Desc: "a hello without its pid", line: "hello 1"},
		{Desc: "a hello with a field too many", line: "hello 1 4242 x"},
		{Desc: "a hello with a word for its version", line: "hello one 4242"},
		{Desc: "a hello with a signed pid", line: "hello 1 -4242"},
		{Desc: "a hello of version zero", line: "hello 0 4242"},
		{Desc: "a hello with pid zero", line: "hello 1 0"},
		{Desc: "a hello spaced twice", line: "hello  1 4242"},
		{Desc: "a hello with a trailing space", line: "hello 1 4242 "},
		{Desc: "a budget without a duration", line: "budget"},
		{Desc: "a budget that is no duration", line: "budget soon"},
		{Desc: "a negative budget", line: "budget -1s"},
		{Desc: "a stopped without a reason", line: "stopped"},
		{Desc: "a stopped with an empty reason", line: "stopped "},
		{Desc: "a reason of two words", line: "stopped by signal"},
		{Desc: "a reason that is not ASCII", line: "stopped caf\xc3\xa9"},
		{Desc: "a stop with an argument", line: "stop now"},
		{Desc: "a stop with a carriage return", line: "stop\r"},
		{Desc: "a verb in capitals", line: "STOP"},
		{Desc: "an unknown verb", line: "ping"},
		{Desc: "an empty line", line: ""},
	}
	for t, e := range gotest.Each(t, entries) {
		got, ok := control.Parse(e.line)
		gotest.False(t, ok, "%q parsed as %+v", e.line, got)
	}
}

func (s *ProtocolTestSuite) TestSplit(t *gotest.T) {
	longest := "stopped " + strings.Repeat("r", control.MaxLine-1-len("stopped "))
	entries := []struct {
		Desc   string
		up     bool
		chunks []string
		want   []control.Line
	}{
		{
			Desc:   "a line too long is dropped and the next one read",
			up:     true,
			chunks: []string{"stopped " + strings.Repeat("r", 292) + "\nbudget 1s\n"},
			want:   []control.Line{{Kind: control.Budget, Duration: time.Second}},
		},
		{
			Desc:   "a line too long is dropped up to its newline across reads",
			chunks: []string{strings.Repeat("a", 300), "stop\n", "force\n"},
			want:   []control.Line{{Kind: control.Force}},
		},
		{
			Desc:   "a line of the longest length is read",
			up:     true,
			chunks: []string{longest + "\n"},
			want:   []control.Line{{Kind: control.Stopped, Reason: longest[len("stopped "):]}},
		},
		{
			Desc:   "a line one byte longer is not",
			up:     true,
			chunks: []string{longest + "r\nbudget 2s\n"},
			want:   []control.Line{{Kind: control.Budget, Duration: 2 * time.Second}},
		},
		{
			Desc:   "a line split across reads",
			up:     true,
			chunks: []string{"hel", "lo 1 7", "\n"},
			want:   []control.Line{{Kind: control.Hello, Version: 1, PID: 7}},
		},
		{
			Desc:   "a partial line waits for its newline",
			chunks: []string{"stop\nfor"},
			want:   []control.Line{{Kind: control.Stop}},
		},
		{
			Desc:   "unknown and malformed lines are ignored",
			chunks: []string{"ping\nstop now\nstop\n"},
			want:   []control.Line{{Kind: control.Stop}},
		},
		{
			Desc:   "the runner keeps only what a suite process sends",
			up:     true,
			chunks: []string{"stop\nforce\nhello 1 9\n"},
			want:   []control.Line{{Kind: control.Hello, Version: 1, PID: 9}},
		},
		{
			Desc:   "a suite process keeps only what the runner sends",
			chunks: []string{"hello 1 9\nbudget 1s\nstopped runner-gone\nforce\n"},
			want:   []control.Line{{Kind: control.Force}},
		},
	}
	for t, e := range gotest.Each(t, entries) {
		gotest.Equal(t, e.want, control.Split(e.up, e.chunks...))
	}
}

func (s *ProtocolTestSuite) TestFormat(t *gotest.T) {
	entries := []struct {
		Desc string
		line control.Line
		want string
	}{
		{Desc: "a hello", line: control.Line{Kind: control.Hello, Version: control.Version, PID: 4242}, want: "hello 1 4242\n"},
		{Desc: "a budget", line: control.Line{Kind: control.Budget, Duration: 5*time.Minute + 30*time.Second}, want: "budget 5m30s\n"},
		{Desc: "a budget in microseconds, in ASCII", line: control.Line{Kind: control.Budget, Duration: 1500 * time.Nanosecond}, want: "budget 1.5us\n"},
		{Desc: "a stopped", line: control.Line{Kind: control.Stopped, Reason: "runner-gone"}, want: "stopped runner-gone\n"},
		{Desc: "a stop", line: control.Line{Kind: control.Stop}, want: "stop\n"},
		{Desc: "a force", line: control.Line{Kind: control.Force}, want: "force\n"},
	}
	for t, e := range gotest.Each(t, entries) {
		got, err := control.Format(e.line)
		gotest.NoError(t, err)
		gotest.Equal(t, e.want, got)
		back, ok := control.Parse(strings.TrimSuffix(got, "\n"))
		gotest.True(t, ok, "%q does not parse back", got)
		gotest.Equal(t, e.line, back)
	}
}

func (s *ProtocolTestSuite) TestFormat_Refused(t *gotest.T) {
	entries := []struct {
		Desc string
		line control.Line
	}{
		{Desc: "a reason of two words", line: control.Line{Kind: control.Stopped, Reason: "by signal"}},
		{Desc: "an empty reason", line: control.Line{Kind: control.Stopped}},
		{Desc: "a reason with a newline", line: control.Line{Kind: control.Stopped, Reason: "a\nstop"}},
		{Desc: "a reason too long for a line", line: control.Line{Kind: control.Stopped, Reason: strings.Repeat("r", control.MaxLine)}},
		{Desc: "a negative budget", line: control.Line{Kind: control.Budget, Duration: -time.Second}},
		{Desc: "a hello without a pid", line: control.Line{Kind: control.Hello, Version: control.Version}},
		{Desc: "no kind", line: control.Line{}},
	}
	for t, e := range gotest.Each(t, entries) {
		_, err := control.Format(e.line)
		gotest.Error(t, err)
	}
}
