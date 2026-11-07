// Package stopbudget is a fixture-bound suite whose process lingers after
// its tests: a stopped run of it shows how long the runner waits for it.
package stopbudget

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
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

// BudgetFixture declares the Timeout STOPBUDGET_TIMEOUT names, 1s without
// it: the teardown budget the suite reports grows with it.
type BudgetFixture struct{}

func (f *BudgetFixture) FixtureConfig() gotest.FixtureConfig {
	d, err := time.ParseDuration(os.Getenv("STOPBUDGET_TIMEOUT"))
	if err != nil {
		d = time.Second
	}
	return gotest.FixtureConfig{Timeout: d}
}

func (f *BudgetFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BudgetFixture) AfterAll(ctx context.Context) error  { return nil }
