package withembedding

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{}

func (f *DBFixture) BeforeAll(context.Context) error { return nil }

type BaseTestSuite struct{}

func (s *BaseTestSuite) TestBase(t *gotest.T) {}

// ToolsTestSuite declares no case: a helper, not a suite.
type ToolsTestSuite struct{}

func (s *ToolsTestSuite) Do() {}

type hooks struct{}

func (h *hooks) BeforeEach(t *gotest.T) {}

type plain struct{ n int }

func (p *plain) bump() { p.n++ }

type ChildTestSuite struct {
	*DBFixture    // want `suite ChildTestSuite embeds fixture DBFixture; fixtures compose through a named pointer field`
	BaseTestSuite // want `suite ChildTestSuite embeds suite BaseTestSuite; suites are never embedded — share behavior through a generic contract suite`
	hooks         // want `suite ChildTestSuite embeds hooks, whose method BeforeEach gotest would read; keep helpers in a named field or in functions`
	plain
	ToolsTestSuite
	Inner *BaseTestSuite // want `suite ChildTestSuite has a field of suite type BaseTestSuite; suites are never nested — share behavior through a generic contract suite`
	Tools ToolsTestSuite
	Cache *CacheFixture
	DB    *DBFixture
}

func (s *ChildTestSuite) TestChild(t *gotest.T) { s.bump() }

type CacheFixture struct {
	*DBFixture // want `fixture CacheFixture embeds fixture DBFixture; fixtures compose through a named pointer field`
}

func (f *CacheFixture) BeforeAll(context.Context) error { return nil }

// LooseFixture is bound by no suite: an ordinary type to gotest.
type LooseFixture struct {
	*DBFixture
}
