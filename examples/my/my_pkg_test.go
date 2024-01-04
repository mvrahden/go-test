package my_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

//go:generate github.com/mvrahden/go-test/cmd/testgen -skip-autogen

type X_MySkippedTestSuite struct{}
type MyNoopTestSuite struct{}

type MyTestSuite struct {
	ExecutedC chan string
}

func (s *MyTestSuite) BeforeAll(t *gotest.T) {
	s.ExecutedC = make(chan string, 100)
	s.ExecutedC <- "BeforeAll"
}

func (s *MyTestSuite) BeforeEach(t *gotest.T) {
	s.ExecutedC <- "BeforeEach"
}

func (s *MyTestSuite) X_TestSomethingSpecific(t *gotest.T) {
	s.ExecutedC <- "XTestSomethingSpecific"
} // skip

func (s *MyTestSuite) F_TestSomethingSpecific(t *gotest.T) {
	s.ExecutedC <- "FTestSomethingSpecific"
} // focus

func (s *MyTestSuite) TestSomethingSpecific(t *gotest.T) {
	s.ExecutedC <- "TestSomethingSpecific"
}

// func (s *MyTestSuite) TestSomethingFunction(t *gotest.T) {
// 	s.ExecutedC <- "TestSomethingFunction"
// }

// func (s *MyTestSuite) TestSomethingAny(t *gotest.T) {
// 	s.ExecutedC <- "TestSomethingAny"
// }

// func (s *MyTestSuite) TestSomethingTime(t *gotest.T) {
// 	s.ExecutedC <- "TestSomethingTime"
// }

// func (s *MyTestSuite) TestSomethingDuration(t *gotest.T) {
// 	s.ExecutedC <- "TestSomethingDuration"
// }

// func (s *MyTestSuite) TestSomethingAsync(t *gotest.T, done func()) {
// 	go func() {
// 		s.ExecutedC <- "TestSomethingAsync"
// 		done()
// 	}()
// }

// func (s *MyTestSuite) TestSomethingB(t *gotest.T) {
// 	s.ExecutedC <- "TestSomethingB"
// }

func (s *MyTestSuite) TestParallelSomethingC(t *gotest.T) {
	s.ExecutedC <- "TestParallelSomethingC"
}

func (s *MyTestSuite) TestParallelSomethingD(t *gotest.T) {
	s.ExecutedC <- "TestParallelSomethingD"
}

func (s *MyTestSuite) TestParallelSomethingE(t *gotest.T) {
	s.ExecutedC <- "TestParallelSomethingE"
}

func (s *MyTestSuite) AfterEach(t *gotest.T) {
	s.ExecutedC <- "AfterEach"
}

func (s *MyTestSuite) AfterAll(t *gotest.T) {
	s.ExecutedC <- "AfterAll"
	close(s.ExecutedC)
}
