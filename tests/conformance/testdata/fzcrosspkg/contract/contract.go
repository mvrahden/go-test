// Package contract holds a generic suite with fuzz methods, for packages
// that alias it.
package contract

import "github.com/mvrahden/go-test/pkg/gotest"

// Count is a named type the fuzz targets take.
type Count int

// CountContractTestSuite is a contract every instantiation fuzzes.
type CountContractTestSuite[V ~int] struct{}

func (s *CountContractTestSuite[V]) TestOne(t *gotest.T) {}

func (s *CountContractTestSuite[V]) FuzzCount(f *gotest.F) {
	f.Add("seed", 3)
	f.Fuzz(func(t *gotest.T, in string, n int) {
		gotest.Equal(t, "seed", in)
		gotest.Equal(t, 3, n)
	})
}

// FuzzValue takes the suite's type parameter, as the alias instantiates it.
func (s *CountContractTestSuite[V]) FuzzValue(f *gotest.F) {
	f.Add(V(4))
	f.Fuzz(func(t *gotest.T, v V) {
		gotest.Equal(t, V(4), v)
	})
}

// FuzzNamed takes a named type this package declares.
func (s *CountContractTestSuite[V]) FuzzNamed(f *gotest.F) {
	f.Add(Count(5))
	f.Fuzz(func(t *gotest.T, c Count) {
		gotest.Equal(t, Count(5), c)
	})
}
