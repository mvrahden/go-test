// Package lcrrelay has suites whose output the runner relays while something
// goes wrong beside it: a stop with no channel to take it, a converter that
// dies mid-run, and a process that crashes under its method.
package lcrrelay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

var markMu sync.Mutex

// mark appends one line to the markers file the run under test reads back.
func mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// HoldFixture records its setup, its release and the process it lives in.
type HoldFixture struct{}

func (f *HoldFixture) BeforeAll(ctx context.Context) error {
	mark("fixture.BeforeAll")
	mark(fmt.Sprintf("pid=%d", os.Getpid()))
	mark(fmt.Sprintf("pgid=%d", syscall.Getpgrp()))
	return nil
}

func (f *HoldFixture) AfterAll(ctx context.Context) error {
	mark("fixture.AfterAll")
	return nil
}
