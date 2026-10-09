// Package quick is stopped by the lifecycle suite; see stopping_suite_test.go.
package quick

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
	if on("STOPPING_PIPE") {
		out, err := runPipeline(ctx)
		if err != nil {
			return err
		}
		logLine("pipe " + out)
	}
	logLine("db down")
	return nil
}
