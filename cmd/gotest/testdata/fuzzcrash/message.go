// Package fuzzcrash is the module the crasher-loop and triage suites stage
// outside the repository: a user's module with one target that crashes on
// purpose, one whose planted crasher no longer fails, and one struct target.
package fuzzcrash

import "fmt"

type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
)

type Message struct {
	To       string
	Subject  string
	Body     string
	Priority Priority
}

func Summary(m Message) string {
	label := "LOW"
	switch m.Priority {
	case PriorityHigh:
		label = "HIGH"
	case PriorityNormal:
		label = "NORMAL"
	}
	return fmt.Sprintf("[%s] %s → %s", label, m.Subject, m.To)
}
