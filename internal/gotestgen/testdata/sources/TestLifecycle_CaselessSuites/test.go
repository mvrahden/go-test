package testpkg

import (
	"context"
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MarkFixture struct{}

func (f *MarkFixture) BeforeAll(ctx context.Context) error {
	fmt.Println("MARK:fixture beforeall")
	return nil
}

type BenchOnlyTestSuite struct{}

func (s *BenchOnlyTestSuite) BeforeAll(t *gotest.T) { fmt.Println("MARK:bench beforeall") }
func (s *BenchOnlyTestSuite) AfterAll(t *gotest.T)  {}
func (s *BenchOnlyTestSuite) BenchmarkNoop(b *gotest.B) {
	for b.Loop() {
	}
}

type BoundBenchTestSuite struct {
	*MarkFixture
}

func (s *BoundBenchTestSuite) BenchmarkNoop(b *gotest.B) {
	for b.Loop() {
	}
}

type FuzzOnlyTestSuite struct{}

func (s *FuzzOnlyTestSuite) BeforeAll(t *gotest.T) { fmt.Println("MARK:fuzz beforeall") }
func (s *FuzzOnlyTestSuite) AfterAll(t *gotest.T)  {}
func (s *FuzzOnlyTestSuite) FuzzEcho(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) { fmt.Println("MARK:fuzz seed", in) })
}

type AllExcludedTestSuite struct{}

func (s *AllExcludedTestSuite) BeforeAll(t *gotest.T) { fmt.Println("MARK:excluded beforeall") }
func (s *AllExcludedTestSuite) AfterAll(t *gotest.T)  {}
func (s *AllExcludedTestSuite) X_TestOne(t *gotest.T) {}

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestOne(t *gotest.T) {}
