package lintsoft

import (
	"errors"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

var errBoom = errors.New("boom")

func check(t *gotest.T) { gotest.True(t, true) }

type SoftTestSuite struct{}

func (s *SoftTestSuite) TestPoll(t *gotest.T) {
	gotest.Eventually(t, time.Second, time.Millisecond, func(poll *gotest.R) {
		gotest.Equal(t.Soft(), 1, 1)
		check(t.Soft())
		t.Soft().Errorf("late")
	})
}

func (s *SoftTestSuite) TestRedundant(t *gotest.T) {
	err := errBoom
	gotest.Error(t, err)
	gotest.ErrorIs(t.Soft(), err, errBoom)
	gotest.Error(t.Soft(), err)
	gotest.ErrorIs(t.Soft(), err, errBoom)
}

func (s *SoftTestSuite) TestFailGuard(t *gotest.T) {
	a, b := 1, 2
	if a == b {
		gotest.Fail(t.Soft(), "same")
	}
	if a > b || b > 3 {
		gotest.Fail(t.Soft(), "out of order")
	}
	if a == b {
		t.Soft().FailNow()
	}
	if a == b {
		t.Soft().Errorf("same")
	}
}

func (s *SoftTestSuite) TestSimplify(t *gotest.T) {
	a, b := 1, 1
	gotest.True(t.Soft(), a == b)
	var p *int
	gotest.Nil(t.Soft(), p)
}

type SoftBenchTestSuite struct{}

func (s *SoftBenchTestSuite) BeforeEach(t *gotest.T) { gotest.NoError(t.Soft(), nil) }

func (s *SoftBenchTestSuite) AfterEach(t *gotest.T) { _ = t.Soft() }

func (s *SoftBenchTestSuite) BenchmarkLoop(b *gotest.B) {
	for b.Loop() {
	}
}

type SoftFuzzTestSuite struct{}

func (s *SoftFuzzTestSuite) BeforeAll(t *gotest.T) { gotest.NoError(t.Soft(), nil) }

func (s *SoftFuzzTestSuite) AfterAll(t *gotest.T) { gotest.NoError(t.Soft(), nil) }

func (s *SoftFuzzTestSuite) FuzzInput(f *gotest.F) {
	f.Add(1)
	f.Fuzz(func(t *gotest.T, n int) {
		gotest.Equal(t.Soft(), n, n)
	})
}
