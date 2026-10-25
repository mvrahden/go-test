package proctree_test

import (
	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ExitStatusTestSuite covers how an exit status is spelled.
type ExitStatusTestSuite struct{}

func (s *ExitStatusTestSuite) TestExitStatus(t *gotest.T) {
	entries := []struct {
		Desc   string
		status int
		want   string
	}{
		{Desc: "a console interrupt's termination status", status: 3221225786, want: "0xC000013A (STATUS_CONTROL_C_EXIT)"},
		{Desc: "a termination status without a name", status: 0xC0000999, want: "0xC0000999"},
		{Desc: "a code the process chose", status: 2, want: "2"},
		{Desc: "the highest code a process chooses", status: 255, want: "255"},
		{Desc: "a signal os/exec reports as -1", status: -1, want: "-1"},
	}
	for t, e := range gotest.Each(t, entries) {
		gotest.Equal(t, e.want, proctree.ExitStatus(e.status))
	}
}
