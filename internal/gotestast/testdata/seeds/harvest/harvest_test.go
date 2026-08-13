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

// MsgTestSuite exists to pin the harvester's native-only invariant:
// FuzzHandleMsg's callback takes a struct, and both tests below feed
// HandleMsg composite literals that LOOK harvestable. None of them may
// harvest — see SeedsTestSuite's struct-target assertion for why.
type MsgTestSuite struct{}

func (s *MsgTestSuite) TestHandleMsgTable(t *gotest.T) {
	type msgCase struct {
		Desc string
		In   Msg
	}
	for t, tc := range gotest.Each(t, []msgCase{
		{"empty", Msg{Text: ""}},
		{"short", Msg{Text: "hi"}},
	}) {
		t.It("handles", func(t *gotest.T) {
			HandleMsg(tc.In)
		})
	}
}

func (s *MsgTestSuite) TestHandleMsgDirect(t *gotest.T) {
	t.It("handles a composite literal directly", func(t *gotest.T) {
		HandleMsg(Msg{Text: "direct"})
	})
}

func (s *MsgTestSuite) FuzzHandleMsg(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in Msg) {
		HandleMsg(in)
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
