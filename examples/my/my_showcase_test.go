package my_test

// import (
// 	"time"

// 	"github.com/mvrahden/go-test/pkg/gotest"
// )

// //go:generate github.com/mvrahden/go-test/cmd/testgen -auto-gen

// // `-auto-gen` induces a re-generation upon test-execution.

// type MyTestSuite struct{}

// func (s *MyTestSuite) BeforeAll(t *gotest.T)              {}
// func (s *MyTestSuite) BeforeEach(t *gotest.T)             {}
// func (s *MyTestSuite) xTestSomethingSpecific(t *gotest.T) {} // skip
// func (s *MyTestSuite) fTestSomethingSpecific(t *gotest.T) {} // focus
// func (s *MyTestSuite) TestSomethingSpecificScopes(t *gotest.T) { // scopes
// 	t.It("should run with ...", func(t *gotest.T) {
// 		// do something
// 	})
// 	t.XIt("should run with ...", func(t *gotest.T) { // skip
// 		// do something
// 	})
// 	t.FIt("should run with ...", func(t *gotest.T) { // focus
// 		// do something
// 	})
// 	t.ItAsync("should run with ...", func(t *gotest.T, done func()) {
// 		go func() {
// 			// do something....
// 			done()
// 		}()
// 	})
// 	t.XItAsync("should run with ...", func(t *gotest.T, done func()) { // skip
// 		go func() {
// 			// do something....
// 			done()
// 		}()
// 	})
// 	t.FItAsync("should run with ...", func(t *gotest.T, done func()) { // focus
// 		go func() {
// 			// do something....
// 			done()
// 		}()
// 	})
// }

// func (s *MyTestSuite) TestSomethingSpecific(t *gotest.T) {
// 	// given
// 	expected := struct{ ABC string }{}
// 	// when
// 	actual := time.Time{}
// 	// then
// 	t.Assert.Not.Zero(actual)
// 	t.Assert.Not.Equal(actual, expected)
// }
// func (s *MyTestSuite) TestSomethingFunction(t *gotest.T) {
// 	// given
// 	// when
// 	actual := time.Time{}
// 	// then
// 	t.Assert.Not.Zero(actual)
// 	t.Assert.Not.Equal(actual, expected)
// }

// func (s *MyTestSuite) TestSomethingAny(t *gotest.T) {
// 	// given
// 	expected := time.Now()
// 	// when
// 	actual := time.Time{}
// 	// then
// 	t.Assert.Any(actual).Not.Zero()
// 	t.Assert.Any(actual).Not.OfType(time.Time{})
// 	t.Assert.Any(actual).Not.EqualTo(expected)
// }

// func (s *MyTestSuite) TestSomethingTime(t *gotest.T) {
// 	// given
// 	expected := time.Now()
// 	// when
// 	actual := time.Time{}
// 	// then
// 	t.Assert.Time(actual).NotZero()
// 	t.Assert.Time(actual).EqualTo(expected, gotest.Within(1*time.Second))

// 	// when
// 	actual := time.Duration{}
// 	// then
// 	t.Assert.Duration(actual).NotZero()
// 	t.Assert.Duration(actual).EqualTo(expected)
// }

// func (s *MyTestSuite) TestSomethingDuration(t *gotest.T) {
// 	// given
// 	expected := 1 * time.Second
// 	// when
// 	actual := time.Minute
// 	// then
// 	t.Assert.Duration(actual).NotZero()
// 	t.Assert.Duration(actual).GreaterThen(1 * time.Second)
// 	t.Assert.Duration(actual).LessThen(1 * time.Second)
// 	t.Assert.Duration(actual).EqualTo(1 * time.Second)
// }

// func (s *MyTestSuite) TestSomethingB(t *gotest.T) {
// 	actual := 1
// 	t.Assert.Comparable(actual).GreaterThan(2)
// 	t.Assert.Number(actual).EqualTo(10)

// 	t.Assert.GJSON(actual).HasKey("abc.def.ghi").String.EqualTo("xyz")
// }

// func (s *MyTestSuite) TestParallelSomethingC(t *gotest.T) {
// 	actual := `{"abc":{"def":{"ghi":123}}}`
// 	t.Assert.GJSON(actual).HasKey("abc.def.ghi").Exists()
// 	t.Assert.GJSON(actual).HasKey("abc.def.ghi").IsString()
// 	t.Assert.GJSON(actual).HasKey("abc.def.ghi").String.EqualTo("xyz")
// }

// func (s *MyTestSuite) TestParallelSomethingD(t *gotest.T) {}
// func (s *MyTestSuite) TestParallelSomethingE(t *gotest.T) {}
// func (s *MyTestSuite) AfterEach(t *gotest.T)              {}
// func (s *MyTestSuite) AfterAll(t *gotest.T)               {}
