package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type WithMap struct {
	Headers map[string]string
}

type MapFuzzTestSuite struct{}

func (s *MapFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *MapFuzzTestSuite) FuzzMap(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, m WithMap) {
		gotest.True(t, len(m.Headers) >= 0)
	})
}
