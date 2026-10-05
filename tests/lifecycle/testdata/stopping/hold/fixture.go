// Package hold is stopped by the lifecycle suite; see stopping_suite_test.go.
package hold

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

func logLine(event string) {
	f, err := os.OpenFile(os.Getenv("STOPPING_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Fprintln(f, event)
}

func on(name string) bool { return os.Getenv(name) == "1" }

// DBFixture comes up at once.
type DBFixture struct{}

func (f *DBFixture) BeforeAll(ctx context.Context) error {
	logLine("db up")
	return nil
}

func (f *DBFixture) AfterAll(ctx context.Context) error {
	if on("STOPPING_SLOW_TEARDOWN") {
		logLine("db tearing")
		time.Sleep(2 * time.Second)
	}
	logLine("db down")
	return nil
}

// PoolFixture hands out one connection; its teardown waits for it back, as
// closing a pool waits for connections in use.
type PoolFixture struct {
	DB   *DBFixture
	conn chan struct{}
}

func (f *PoolFixture) BeforeAll(ctx context.Context) error {
	f.conn = make(chan struct{}, 1)
	f.conn <- struct{}{}
	return nil
}

func (f *PoolFixture) Acquire() { <-f.conn }
func (f *PoolFixture) Release() { f.conn <- struct{}{} }

func (f *PoolFixture) AfterAll(ctx context.Context) error {
	select {
	case <-f.conn:
		logLine("pool down")
		return nil
	case <-time.After(60 * time.Second):
		return errors.New("a connection was never returned")
	}
}
