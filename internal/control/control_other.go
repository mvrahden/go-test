//go:build aix || (!unix && !windows)

package control

import (
	"errors"
	"os"
	"os/exec"
)

var errUnsupported = errors.New("control: no channel on this platform")

// ChildEnd is the end a suite process inherits.
type ChildEnd struct{}

// New fails: this platform has no channel, and the runner falls back.
func New() (*Link, *ChildEnd, error) { return nil, nil, errUnsupported }

// Attach does nothing here.
func (c *ChildEnd) Attach(*exec.Cmd) {}

// Close does nothing here.
func (c *ChildEnd) Close() {}

func claim(string) *Endpoint { return nil }

func readNow(*os.File, []byte) (int, bool) { return 0, true }
