package stopbench

import (
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchStopTestSuite's first benchmark runs while the run is stopped.
type BenchStopTestSuite struct {
	Hold *HoldFixture
}

func (s *BenchStopTestSuite) AfterAll(t *gotest.T) { mark("suite.AfterAll") }

func (s *BenchStopTestSuite) BenchmarkFirst(b *gotest.B) {
	mark("running")
	if os.Getenv("STOPBENCH_IGNORE") == "1" {
		time.Sleep(time.Minute)
	} else {
		time.Sleep(2 * time.Second)
	}
	mark("first returned")
	for b.Loop() {
	}
}

func (s *BenchStopTestSuite) BenchmarkSecond(b *gotest.B) {
	mark("second ran")
	for b.Loop() {
	}
}

// SeedStopTestSuite's first seed runs while the run is stopped.
type SeedStopTestSuite struct {
	Hold *HoldFixture
}

func (s *SeedStopTestSuite) AfterAll(t *gotest.T) { mark("suite.AfterAll") }

func (s *SeedStopTestSuite) FuzzSeeds(f *gotest.F) {
	f.Add(1)
	f.Add(2)
	f.Fuzz(func(t *gotest.T, n int) {
		if n != 1 {
			mark("second seed ran")
			return
		}
		mark("running")
		time.Sleep(2 * time.Second)
		mark("first seed returned")
	})
}
