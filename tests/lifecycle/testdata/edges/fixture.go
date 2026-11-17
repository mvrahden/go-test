package edges

import (
	"context"
	"errors"
	"os"
)

// EdgeFixture fails to come up or to let go when EDGES_FAIL says so.
type EdgeFixture struct{}

func (f *EdgeFixture) BeforeAll(ctx context.Context) error {
	if os.Getenv("EDGES_FAIL") == "setup" {
		return errors.New("the edge fixture would not come up")
	}
	return nil
}

func (f *EdgeFixture) AfterAll(ctx context.Context) error {
	if os.Getenv("EDGES_FAIL") == "teardown" {
		return errors.New("the edge fixture would not let go")
	}
	return nil
}
