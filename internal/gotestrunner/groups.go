package gotestrunner

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

// groupSelection is what --group and --skip-group leave of a run's groups.
// The zero value selects every group.
type groupSelection struct {
	only, skip map[string]bool
}

func (g groupSelection) selects(group string) bool {
	if len(g.only) > 0 && !g.only[group] {
		return false
	}
	return !g.skip[group]
}

// knownGroups lists the named groups the overlay's suites declare, sorted.
func (o *OverlayResult) knownGroups() []string {
	set := map[string]bool{}
	for _, suites := range o.GroupsByPkg {
		for _, group := range suites {
			set[group] = true
		}
	}
	return slices.Sorted(maps.Keys(set))
}

// selectGroups narrows the run to the groups only names, less those skip
// names. A name no suite declares is an error naming the known groups.
func (o *OverlayResult) selectGroups(only, skip []string) error {
	known := o.knownGroups()
	for _, flag := range []struct {
		name  string
		names []string
	}{{"--group", only}, {"--skip-group", skip}} {
		for _, name := range flag.names {
			if slices.Contains(known, name) {
				continue
			}
			list := "none, no suite declares an ExclusiveGroup"
			if len(known) > 0 {
				list = strings.Join(known, ", ")
			}
			return fmt.Errorf("%s=%s: no suite declares the group %q; known groups: %s", flag.name, name, name, list)
		}
	}
	sel := groupSelection{}
	if len(only) > 0 {
		sel.only = map[string]bool{}
		for _, name := range only {
			sel.only[name] = true
		}
	}
	if len(skip) > 0 {
		sel.skip = map[string]bool{}
		for _, name := range skip {
			sel.skip[name] = true
		}
	}
	o.groups = sel
	return nil
}

// inSelectedGroups gives each target its group and drops those of a group
// the selection leaves out.
func inSelectedGroups(targets []SuiteTarget, o *OverlayResult) []SuiteTarget {
	kept := targets[:0]
	for i := range targets {
		t := targets[i]
		t.Group = o.GroupsByPkg[t.Package][strings.TrimPrefix(t.SuiteName, "Test")]
		if o.groups.selects(t.Group) {
			kept = append(kept, t)
		}
	}
	return kept
}

// GroupRun is what a named group took in a run: its suites, and the wall
// time from its first suite's start to its last suite's end.
type GroupRun struct {
	Name   string
	Suites int
	Wall   time.Duration
}

// groupClock measures each named group's suites as they run.
type groupClock struct {
	mu     sync.Mutex
	groups map[string]*groupSpan
}

type groupSpan struct {
	suites      int
	first, last time.Time
}

func (c *groupClock) span(group string) *groupSpan {
	if c.groups == nil {
		c.groups = map[string]*groupSpan{}
	}
	s := c.groups[group]
	if s == nil {
		s = &groupSpan{}
		c.groups[group] = s
	}
	return s
}

// count books targets of a named group.
func (c *groupClock) count(targets []SuiteTarget) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range targets {
		if targets[i].Group != "" {
			c.span(targets[i].Group).suites++
		}
	}
}

// run times fn, a suite of group running.
func (c *groupClock) run(group string, fn func()) {
	start := time.Now()
	fn()
	if group == "" {
		return
	}
	end := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.span(group)
	if s.first.IsZero() || start.Before(s.first) {
		s.first = start
	}
	if end.After(s.last) {
		s.last = end
	}
}

// runs is the GroupRun of every named group a suite of which ran, by name.
func (c *groupClock) runs() []GroupRun {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]GroupRun, 0, len(c.groups))
	for _, name := range slices.Sorted(maps.Keys(c.groups)) {
		s := c.groups[name]
		if s.first.IsZero() {
			continue
		}
		out = append(out, GroupRun{Name: name, Suites: s.suites, Wall: s.last.Sub(s.first)})
	}
	return out
}
