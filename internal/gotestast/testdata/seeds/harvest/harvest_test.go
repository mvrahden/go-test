package harvest

import "github.com/mvrahden/go-test/pkg/gotest"

type ParseTestSuite struct{}

func (s *ParseTestSuite) TestParseTable(t *gotest.T) {
	type parseCase struct {
		Desc  string
		Input string
	}
	for t, tc := range gotest.Each(t, []parseCase{
		{"single digit", "5"},
		{"double digit", "42"},
		{"computed at runtime", computedInput},
	}) {
		t.It("parses", func(t *gotest.T) {
			Parse(tc.Input)
		})
	}
}

func (s *ParseTestSuite) TestParseLiteral(t *gotest.T) {
	t.It("parses a literal directly", func(t *gotest.T) {
		Parse("literal input")
	})
}

func (s *ParseTestSuite) FuzzParse(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		Parse(in)
	})
}

type EchoTestSuite struct{}

func (s *EchoTestSuite) FuzzEchoString(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {
		Echo(in)
	})
}

func (s *EchoTestSuite) FuzzEchoInt(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in int) {
		Echo(in)
	})
}

func (s *EchoTestSuite) TestEchoLiteral(t *gotest.T) {
	t.It("calls Echo with a string literal", func(t *gotest.T) {
		Echo("only a string")
	})
}
