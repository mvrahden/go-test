package fuzzfan

import "github.com/mvrahden/go-test/pkg/gotest"

type RichFuzzTestSuite struct{}

func (s *RichFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *RichFuzzTestSuite) FuzzRich(f *gotest.F) {
	f.Add(Rich{Name: "seed"})
	gotest.Fuzz(f, func(t *gotest.T, v Rich) {
		gotest.True(t, len(v.Name) >= 0)
	})
}
