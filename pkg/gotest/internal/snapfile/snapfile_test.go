package snapfile_test //nolint:stdlib-test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotest/internal/snapfile"
)

func TestParse_EmptyInput(t *testing.T) {
	sections := snapfile.Parse([]byte{})
	gotest.Empty(t, sections)
}

func TestParse_SingleSection(t *testing.T) {
	input := []byte("=== SNAP my-key ===\nhello world\n")
	sections := snapfile.Parse(input)
	gotest.Len(t, sections, 1)
	gotest.Equal(t, "my-key", sections[0].Key)
	gotest.Equal(t, "hello world\n", sections[0].Content)
}

func TestParse_MultipleSections(t *testing.T) {
	input := []byte("=== SNAP alpha ===\nfirst\n=== SNAP beta ===\nsecond\n")
	sections := snapfile.Parse(input)
	gotest.Len(t, sections, 2)
	gotest.Equal(t, "alpha", sections[0].Key)
	gotest.Equal(t, "first\n", sections[0].Content)
	gotest.Equal(t, "beta", sections[1].Key)
	gotest.Equal(t, "second\n", sections[1].Content)
}

func TestParse_MultilineContent(t *testing.T) {
	input := []byte("=== SNAP key ===\nline1\nline2\nline3\n")
	sections := snapfile.Parse(input)
	gotest.Len(t, sections, 1)
	gotest.Equal(t, "line1\nline2\nline3\n", sections[0].Content)
}

func TestParse_ContentBeforeFirstHeader_IsIgnored(t *testing.T) {
	input := []byte("stray line\n=== SNAP key ===\ncontent\n")
	sections := snapfile.Parse(input)
	gotest.Len(t, sections, 1)
	gotest.Equal(t, "key", sections[0].Key)
	gotest.Equal(t, "content\n", sections[0].Content)
}

func TestSerialize_Empty(t *testing.T) {
	out := snapfile.Serialize(nil)
	gotest.Equal(t, "", string(out))
}

func TestSerialize_SingleSection(t *testing.T) {
	sections := []snapfile.Section{{Key: "key", Content: "hello\n"}}
	out := snapfile.Serialize(sections)
	gotest.Equal(t, "=== SNAP key ===\nhello\n", string(out))
}

func TestSerialize_MultipleSections_SortedByKey(t *testing.T) {
	sections := []snapfile.Section{
		{Key: "beta", Content: "second\n"},
		{Key: "alpha", Content: "first\n"},
	}
	out := snapfile.Serialize(sections)
	expected := "=== SNAP alpha ===\nfirst\n=== SNAP beta ===\nsecond\n"
	gotest.Equal(t, expected, string(out))
}

func TestSerialize_MultilineContent(t *testing.T) {
	sections := []snapfile.Section{{Key: "key", Content: "line1\nline2\n"}}
	out := snapfile.Serialize(sections)
	gotest.Equal(t, "=== SNAP key ===\nline1\nline2\n", string(out))
}

func TestRoundTrip_ParseThenSerialize(t *testing.T) {
	input := "=== SNAP alpha ===\nfirst\n=== SNAP beta ===\nsecond\nthird\n"
	sections := snapfile.Parse([]byte(input))
	out := snapfile.Serialize(sections)
	gotest.Equal(t, input, string(out))
}

func TestValidateContent_Clean(t *testing.T) {
	err := snapfile.ValidateContent("normal content\nmore lines\n")
	gotest.NoError(t, err)
}

func TestValidateContent_ContainsHeader(t *testing.T) {
	err := snapfile.ValidateContent("line\n=== SNAP injected ===\nmore\n")
	gotest.Error(t, err)
}

func TestValidateContent_HeaderOnly(t *testing.T) {
	err := snapfile.ValidateContent("=== SNAP injected ===")
	gotest.Error(t, err)
}

func TestValidateContent_PartialHeaderMatch(t *testing.T) {
	err := snapfile.ValidateContent("=== SNAP foo ===extra")
	gotest.NoError(t, err)
}

func TestValidateContent_IncompleteHeader(t *testing.T) {
	err := snapfile.ValidateContent("=== SNAP foo")
	gotest.NoError(t, err)
}
