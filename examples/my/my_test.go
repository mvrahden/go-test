package my

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/stretchr/testify/require"
)

type X_MySkippedTestSuite struct{}
type MyNoopTestSuite struct{}
type MyNoopParallelTestSuite struct{}

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
	s.ExecutedC <- "X_TestSomethingSpecific"
} // skip

func (s *MyTestSuite) F_TestSomethingSpecific(t *gotest.T) {
	s.ExecutedC <- "F_TestSomethingSpecific"
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

func (s *MyTestSuite) F_TestParallelSomethingC(t *gotest.T) {
	s.ExecutedC <- "F_TestParallelSomethingC"
}

func (s *MyTestSuite) TestParallelSomethingD(t *gotest.T) {
	s.ExecutedC <- "TestParallelSomethingD"
}

func (s *MyTestSuite) TestParallelSomethingE(t *gotest.T) {
	s.ExecutedC <- "TestParallelSomethingE"
	// require.Contains(t.T(), "actualSequence", "BeforeEach:F_TestSomething\Specific:AfterEach")
}

func (s *MyTestSuite) AfterEach(t *gotest.T) {
	s.ExecutedC <- "AfterEach"
}

func (s *MyTestSuite) AfterAll(t *gotest.T) {
	s.ExecutedC <- "AfterAll"
	close(s.ExecutedC)

	var actualSequence string
	for v := range s.ExecutedC {
		actualSequence += v + ":"
	}
	require.GreaterOrEqual(t.T(), len(actualSequence), 100)
	require.True(t.T(), strings.HasPrefix(actualSequence, "BeforeAll:"))
	require.True(t.T(), strings.HasSuffix(actualSequence, ":AfterAll:"))
	// require.Contains(t.T(), actualSequence, "BeforeEach:TestSomethingAny:AfterEach")
	// require.Contains(t.T(), actualSequence, "BeforeEach:TestSomethingB:AfterEach")
	// require.Contains(t.T(), actualSequence, "BeforeEach:TestSomethingDuration:AfterEach")
	// require.Contains(t.T(), actualSequence, "BeforeEach:TestSomethingFunction:AfterEach")
	// require.Contains(t.T(), actualSequence, "BeforeEach:TestSomethingSpecific:AfterEach")
	// require.Contains(t.T(), actualSequence, "BeforeEach:TestSomethingTime:AfterEach")
	// require.Contains(t.T(), actualSequence, "BeforeEach:FTestSomethingSpecific:AfterEach")
	require.Contains(t.T(), actualSequence, "BeforeEach:F_TestSomethingSpecific:AfterEach")
	require.Contains(t.T(), actualSequence, "BeforeEach:F_TestSomethingSpecific:AfterEach")
	require.NotContains(t.T(), actualSequence, "XTestSomethingSpecific")
	require.GreaterOrEqual(t.T(), len(actualSequence), 100)
	require.Contains(t.T(), actualSequence, "F_TestParallelSomethingC")
	// require.Contains(t.T(), actualSequence, "TestParallelSomethingD")
	// require.Contains(t.T(), actualSequence, "TestParallelSomethingE")
}
