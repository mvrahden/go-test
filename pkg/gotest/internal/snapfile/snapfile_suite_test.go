// Ring 0: raw checks only, so a broken assertion engine cannot pass its own
// tests. No gotest.* assertion may appear in this file. snapfile is what
// MatchSnapshot reads and writes, so its truth sits below the assertions.
package snapfile_test //nolint:fail-guard

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotest/internal/snapfile"
)

// SnapfileTestSuite covers the snapshot file format: parsing, serialization
// with sorted keys, CRLF tolerance, and the header-injection guard.
type SnapfileTestSuite struct{}

func (s *SnapfileTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type snapfileCtx struct{}

func (s *SnapfileTestSuite) BeforeEach(t *gotest.T) *snapfileCtx { return &snapfileCtx{} }

func mustSections(t *gotest.T, got []snapfile.Section, want ...snapfile.Section) {
	if len(got) != len(want) {
		t.Errorf("sections: got %d, want %d: %#v", len(got), len(want), got)
		t.FailNow()
	}
	for i := range want {
		if got[i].Key != want[i].Key {
			t.Errorf("section %d key: got %q, want %q", i, got[i].Key, want[i].Key)
		}
		if got[i].Content != want[i].Content {
			t.Errorf("section %d content: got %q, want %q", i, got[i].Content, want[i].Content)
		}
	}
}

func mustString(t *gotest.T, got, want string) {
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func (s *SnapfileTestSuite) TestParse_EmptyInput(t *gotest.T, _ *snapfileCtx) {
	mustSections(t, snapfile.Parse([]byte{}))
}

func (s *SnapfileTestSuite) TestParse_SingleSection(t *gotest.T, _ *snapfileCtx) {
	mustSections(t, snapfile.Parse([]byte("=== SNAP my-key ===\nhello world\n")),
		snapfile.Section{Key: "my-key", Content: "hello world\n"})
}

func (s *SnapfileTestSuite) TestParse_MultipleSections(t *gotest.T, _ *snapfileCtx) {
	mustSections(t, snapfile.Parse([]byte("=== SNAP alpha ===\nfirst\n=== SNAP beta ===\nsecond\n")),
		snapfile.Section{Key: "alpha", Content: "first\n"},
		snapfile.Section{Key: "beta", Content: "second\n"})
}

func (s *SnapfileTestSuite) TestParse_MultilineContent(t *gotest.T, _ *snapfileCtx) {
	mustSections(t, snapfile.Parse([]byte("=== SNAP key ===\nline1\nline2\nline3\n")),
		snapfile.Section{Key: "key", Content: "line1\nline2\nline3\n"})
}

func (s *SnapfileTestSuite) TestParse_ContentBeforeFirstHeader_IsIgnored(t *gotest.T, _ *snapfileCtx) {
	mustSections(t, snapfile.Parse([]byte("stray line\n=== SNAP key ===\ncontent\n")),
		snapfile.Section{Key: "key", Content: "content\n"})
}

func (s *SnapfileTestSuite) TestSerialize_Empty(t *gotest.T, _ *snapfileCtx) {
	mustString(t, string(snapfile.Serialize(nil)), "")
}

func (s *SnapfileTestSuite) TestSerialize_SingleSection(t *gotest.T, _ *snapfileCtx) {
	out := snapfile.Serialize([]snapfile.Section{{Key: "key", Content: "hello\n"}})
	mustString(t, string(out), "=== SNAP key ===\nhello\n")
}

func (s *SnapfileTestSuite) TestSerialize_MultipleSections_SortedByKey(t *gotest.T, _ *snapfileCtx) {
	out := snapfile.Serialize([]snapfile.Section{
		{Key: "beta", Content: "second\n"},
		{Key: "alpha", Content: "first\n"},
	})
	mustString(t, string(out), "=== SNAP alpha ===\nfirst\n=== SNAP beta ===\nsecond\n")
}

func (s *SnapfileTestSuite) TestSerialize_MultilineContent(t *gotest.T, _ *snapfileCtx) {
	out := snapfile.Serialize([]snapfile.Section{{Key: "key", Content: "line1\nline2\n"}})
	mustString(t, string(out), "=== SNAP key ===\nline1\nline2\n")
}

func (s *SnapfileTestSuite) TestRoundTrip_ParseThenSerialize(t *gotest.T, _ *snapfileCtx) {
	input := "=== SNAP alpha ===\nfirst\n=== SNAP beta ===\nsecond\nthird\n"
	mustString(t, string(snapfile.Serialize(snapfile.Parse([]byte(input)))), input)
}

func (s *SnapfileTestSuite) TestParse_CRLFLineEndings(t *gotest.T, _ *snapfileCtx) {
	mustSections(t, snapfile.Parse([]byte("=== SNAP alpha ===\r\nfirst\r\n=== SNAP beta ===\r\nsecond\r\n")),
		snapfile.Section{Key: "alpha", Content: "first\n"},
		snapfile.Section{Key: "beta", Content: "second\n"})
}

func (s *SnapfileTestSuite) TestValidateContent_Clean(t *gotest.T, _ *snapfileCtx) {
	if err := snapfile.ValidateContent("normal content\nmore lines\n"); err != nil {
		t.Errorf("clean content rejected: %v", err)
	}
}

func (s *SnapfileTestSuite) TestValidateContent_ContainsHeader(t *gotest.T, _ *snapfileCtx) {
	if err := snapfile.ValidateContent("line\n=== SNAP injected ===\nmore\n"); err == nil {
		t.Errorf("an embedded header must be rejected")
	}
}

func (s *SnapfileTestSuite) TestValidateContent_HeaderOnly(t *gotest.T, _ *snapfileCtx) {
	if err := snapfile.ValidateContent("=== SNAP injected ==="); err == nil {
		t.Errorf("a header-only content must be rejected")
	}
}

func (s *SnapfileTestSuite) TestValidateContent_PartialHeaderMatch(t *gotest.T, _ *snapfileCtx) {
	if err := snapfile.ValidateContent("=== SNAP foo ===extra"); err != nil {
		t.Errorf("a partial header match is not a header: %v", err)
	}
}

func (s *SnapfileTestSuite) TestValidateContent_IncompleteHeader(t *gotest.T, _ *snapfileCtx) {
	if err := snapfile.ValidateContent("=== SNAP foo"); err != nil {
		t.Errorf("an incomplete header is not a header: %v", err)
	}
}
