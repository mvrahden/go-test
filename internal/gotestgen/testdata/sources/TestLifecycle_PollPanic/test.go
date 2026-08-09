package testpkg

import (
	"context"
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// PollResourceFixture stands in for anything whose release matters — a
// container, a schema, a temp tree.
type PollResourceFixture struct{}

func (f *PollResourceFixture) BeforeAll(_ context.Context) error {
	fmt.Println("MARK:fixture acquired")
	return nil
}

func (f *PollResourceFixture) AfterAll(_ context.Context) error {
	fmt.Println("MARK:fixture released")
	return nil
}

// PollPanicTestSuite panics inside an Eventually poll function. Record runs that
// function on its own goroutine, so an unrecovered panic there would abort the
// process without running any cleanup at all.
type PollPanicTestSuite struct {
	*PollResourceFixture
}

func (s *PollPanicTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:suite afterall")
}

func (s *PollPanicTestSuite) TestPollPanics(t *gotest.T) {
	gotest.Eventually(t, 500*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
		panic("boom inside the poll function")
	})
}
