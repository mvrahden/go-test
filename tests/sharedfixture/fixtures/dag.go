package fixtures

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// The Dag*SharedFixtures are a small dependency graph for the setup
// process's own tests, which drive them directly; no suite references them.
// DagA to DagD are independent, and DagChild depends on DagA.
//
// EnvDagTimeout is the Timeout each declares, EnvDagSetupDelay and
// EnvDagTeardownDelay how long BeforeAll and AfterAll take, and EnvDagLog a
// file each appends "<name> <hook> begin|end <unix nanoseconds>" lines to.
const (
	EnvDagTimeout       = "GOTEST_TEST_DAG_TIMEOUT"
	EnvDagSetupDelay    = "GOTEST_TEST_DAG_SETUP_DELAY"
	EnvDagTeardownDelay = "GOTEST_TEST_DAG_TEARDOWN_DELAY"
	EnvDagLog           = "GOTEST_TEST_DAG_LOG"
)

type DagASharedFixture struct{}

type DagBSharedFixture struct{}

type DagCSharedFixture struct{}

type DagDSharedFixture struct{}

type DagChildSharedFixture struct {
	Parent *DagASharedFixture
}

func dagConfig() gotest.FixtureConfig {
	cfg := gotest.DefaultFixtureConfig()
	cfg.Retries = 0
	if d, err := time.ParseDuration(os.Getenv(EnvDagTimeout)); err == nil && d > 0 {
		cfg.Timeout = d
	}
	return cfg
}

// dagHook logs the hook's begin and end around the delay env names.
func dagHook(name, hook, delayEnv string) error {
	dagLog(name, hook, "begin")
	if d, err := time.ParseDuration(os.Getenv(delayEnv)); err == nil && d > 0 {
		time.Sleep(d)
	}
	dagLog(name, hook, "end")
	return nil
}

func dagLog(name, hook, event string) {
	path := os.Getenv(EnvDagLog)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	// One write per line: O_APPEND keeps lines from goroutines whole.
	_, _ = fmt.Fprintf(f, "%s %s %s %d\n", name, hook, event, time.Now().UnixNano())
	_ = f.Close()
}

func (f *DagASharedFixture) SharedFixtureConfig() gotest.FixtureConfig { return dagConfig() }
func (f *DagASharedFixture) BeforeAll(ctx context.Context) error {
	return dagHook("DagA", "BeforeAll", EnvDagSetupDelay)
}
func (f *DagASharedFixture) AfterAll(ctx context.Context) error {
	return dagHook("DagA", "AfterAll", EnvDagTeardownDelay)
}

func (f *DagBSharedFixture) SharedFixtureConfig() gotest.FixtureConfig { return dagConfig() }
func (f *DagBSharedFixture) BeforeAll(ctx context.Context) error {
	return dagHook("DagB", "BeforeAll", EnvDagSetupDelay)
}
func (f *DagBSharedFixture) AfterAll(ctx context.Context) error {
	return dagHook("DagB", "AfterAll", EnvDagTeardownDelay)
}

func (f *DagCSharedFixture) SharedFixtureConfig() gotest.FixtureConfig { return dagConfig() }
func (f *DagCSharedFixture) BeforeAll(ctx context.Context) error {
	return dagHook("DagC", "BeforeAll", EnvDagSetupDelay)
}
func (f *DagCSharedFixture) AfterAll(ctx context.Context) error {
	return dagHook("DagC", "AfterAll", EnvDagTeardownDelay)
}

func (f *DagDSharedFixture) SharedFixtureConfig() gotest.FixtureConfig { return dagConfig() }
func (f *DagDSharedFixture) BeforeAll(ctx context.Context) error {
	return dagHook("DagD", "BeforeAll", EnvDagSetupDelay)
}
func (f *DagDSharedFixture) AfterAll(ctx context.Context) error {
	return dagHook("DagD", "AfterAll", EnvDagTeardownDelay)
}

func (f *DagChildSharedFixture) SharedFixtureConfig() gotest.FixtureConfig { return dagConfig() }
func (f *DagChildSharedFixture) BeforeAll(ctx context.Context) error {
	return dagHook("DagChild", "BeforeAll", EnvDagSetupDelay)
}
func (f *DagChildSharedFixture) AfterAll(ctx context.Context) error {
	return dagHook("DagChild", "AfterAll", EnvDagTeardownDelay)
}
