// Package setup is stopped by the lifecycle suite; see stopping_suite_test.go.
package setup

import (
	"context"
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

// ContainerFixture depends on DBFixture and takes long to come up, as pulling
// an image does, honouring its context.
type ContainerFixture struct {
	DB *DBFixture
}

func (f *ContainerFixture) BeforeAll(ctx context.Context) error {
	logLine("container starting")
	select {
	case <-time.After(30 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}
	logLine("container up")
	return nil
}

func (f *ContainerFixture) AfterAll(ctx context.Context) error {
	logLine("container down")
	return nil
}
