package main

import (
	"fmt"
	"time"
)

// phases is where a run's time went. Compile lies inside Run, so the line
// gives each span, never a sum.
type phases struct {
	Discover, Generate, Compile, Run, Report, Total time.Duration
}

// String is the line a --debug run ends with, each span rounded to the
// millisecond.
func (p phases) String() string {
	ms := func(d time.Duration) string { return d.Round(time.Millisecond).String() }
	return fmt.Sprintf("DEBUG: phases: discover=%s generate=%s compile=%s run=%s report=%s total=%s",
		ms(p.Discover), ms(p.Generate), ms(p.Compile), ms(p.Run), ms(p.Report), ms(p.Total))
}

// phaseClock holds the time each phase of a run ended. A run that returns
// early leaves the later marks zero, and their spans with them.
type phaseClock struct {
	start, discovered, generated, piped, ran time.Time
	compile                                  time.Duration
}

// phases reads the spans off the marks, with end as the run's end.
func (c *phaseClock) phases(end time.Time) phases {
	span := func(from, to time.Time) time.Duration {
		if from.IsZero() || to.IsZero() {
			return 0
		}
		return to.Sub(from)
	}
	return phases{
		Discover: span(c.start, c.discovered),
		Generate: span(c.discovered, c.generated),
		Compile:  c.compile,
		Run:      span(c.piped, c.ran),
		Report:   span(c.ran, end),
		Total:    span(c.start, end),
	}
}
