package fuzzharvest

import "github.com/mvrahden/go-test/pkg/gotest"

type HarvestFuzzTestSuite struct{}

func (s *HarvestFuzzTestSuite) TestTrimTable(t *gotest.T) {
	type tc struct {
		Desc  string
		Input string
	}
	for t, c := range gotest.Each(t, []tc{
		{"basic", "hello"},
		{"padded", "  hi  "},
	}) {
		t.It("trims", func(t *gotest.T) {
			trimAll(c.Input)
		})
	}
}

func (s *HarvestFuzzTestSuite) FuzzTrim(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		trimAll(in)
	})
}
