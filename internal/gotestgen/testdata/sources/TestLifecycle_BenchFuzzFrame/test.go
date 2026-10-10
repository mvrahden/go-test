package testpkg

import (
	"context"
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// report prints what ctx holds, rounded so scheduling noise does not show.
func report(label string, ctx context.Context) {
	deadline, ok := ctx.Deadline()
	if !ok {
		fmt.Printf("MARK:%s err=%v no deadline\n", label, ctx.Err())
		return
	}
	fmt.Printf("MARK:%s err=%v deadline %s\n", label, ctx.Err(), time.Until(deadline).Round(10*time.Second))
}

// BenchFrameTestSuite declares a SetupTimeout its lifecycle contexts must carry.
type BenchFrameTestSuite struct{}

func (s *BenchFrameTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: 2 * time.Minute}
}

func (s *BenchFrameTestSuite) BeforeAll(t *gotest.T) { report("bench beforeall", t.Context()) }
func (s *BenchFrameTestSuite) AfterAll(t *gotest.T)  { report("bench afterall", t.Context()) }

func (s *BenchFrameTestSuite) BenchmarkNoop(b *gotest.B) {
	for b.Loop() {
	}
}

// FuzzFrameTestSuite declares no config, so its contexts carry the defaults.
type FuzzFrameTestSuite struct{}

func (s *FuzzFrameTestSuite) BeforeAll(t *gotest.T) { report("fuzz beforeall", t.Context()) }
func (s *FuzzFrameTestSuite) AfterAll(t *gotest.T)  { report("fuzz afterall", t.Context()) }

func (s *FuzzFrameTestSuite) FuzzNoop(f *gotest.F) {
	f.Add(1)
	f.Fuzz(func(t *gotest.T, n int) {})
}
