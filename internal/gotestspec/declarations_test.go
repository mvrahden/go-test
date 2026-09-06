package gotestspec //nolint:stdlib-test

import (
	"strings"
	"testing"
)

// The description contains an underscore, so its subtest name cannot be turned
// back into it: go test wrote underscores for the spaces, and nothing in the
// name says which ones were already there.
const declStream = `{"Action":"run","Package":"example.com/pkg","Test":"TestKeysTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestKeysTestSuite/TestEncode"}
{"Action":"run","Package":"example.com/pkg","Test":"TestKeysTestSuite/TestEncode/returns_snake_case_keys"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestKeysTestSuite/TestEncode/returns_snake_case_keys","Elapsed":0.001}
{"Action":"pass","Package":"example.com/pkg","Test":"TestKeysTestSuite/TestEncode","Elapsed":0.002}
{"Action":"pass","Package":"example.com/pkg","Test":"TestKeysTestSuite","Elapsed":0.003}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.004}
`

func declTree(t *testing.T, opts ...BuildOption) *Node {
	t.Helper()
	events, err := ParseEvents(strings.NewReader(declStream))
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	pkgs := BuildTree(events, opts...)
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	return pkgs[0].Nodes[0].Children[0].Children[0]
}

func declsOf(pkg string, entries map[string]Declaration) DeclarationIndex {
	return DeclarationIndex{pkg: entries}
}

func TestBuildTree_ShowsTheDeclaredLabel(t *testing.T) {
	node := declTree(t, WithDeclarations(declsOf("example.com/pkg", map[string]Declaration{
		"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys"},
	})))

	if node.Display != "returns snake_case keys" {
		t.Errorf("display = %q, want %q", node.Display, "returns snake_case keys")
	}
}

// Without a declaration there is nothing to prefer, and reconstructing from the
// name is the honest best effort — the rendering every stream got before.
func TestBuildTree_FallsBackToTheReconstructedLabel(t *testing.T) {
	node := declTree(t)

	if node.Display != "returns snake case keys" {
		t.Errorf("display = %q, want %q", node.Display, "returns snake case keys")
	}
}

// The index is keyed per package: a path that matches in one package must not
// hand its label to an identically-named path in another.
func TestBuildTree_DeclarationsAreScopedToTheirPackage(t *testing.T) {
	node := declTree(t, WithDeclarations(declsOf("example.com/other", map[string]Declaration{
		"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys"},
	})))

	if node.Display != "returns snake case keys" {
		t.Errorf("display = %q, want %q", node.Display, "returns snake case keys")
	}
}

// The label is what a human reads. The name is what -run filters, snapshot keys
// and saved baselines are keyed by, and it must not move.
func TestBuildTree_DeclarationsLeaveNamesAlone(t *testing.T) {
	node := declTree(t, WithDeclarations(declsOf("example.com/pkg", map[string]Declaration{
		"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys"},
	})))

	if node.Name != "returns_snake_case_keys" {
		t.Errorf("name = %q, want %q", node.Name, "returns_snake_case_keys")
	}
}

func TestBuildTree_SpeaksTheDeclaredVocabulary(t *testing.T) {
	node := declTree(t, WithDeclarations(declsOf("example.com/pkg", map[string]Declaration{
		"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys", Vocab: VocabWhen},
	})))

	if node.Display != "when returns snake_case keys" {
		t.Errorf("display = %q, want %q", node.Display, "when returns snake_case keys")
	}
	if node.Vocab != VocabWhen {
		t.Errorf("vocab = %d, want VocabWhen", node.Vocab)
	}
}

// The glyph already plays the role of "it", so an expectation stands on its own.
func TestBuildTree_ExpectationsTakeNoConnective(t *testing.T) {
	node := declTree(t, WithDeclarations(declsOf("example.com/pkg", map[string]Declaration{
		"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys", Vocab: VocabIt},
	})))

	if node.Display != "returns snake_case keys" {
		t.Errorf("display = %q, want %q", node.Display, "returns snake_case keys")
	}
}

// The doubling guard sees the same label as source text and as a subtest name,
// where every space is an underscore. Judging those differently is how one
// behavior gets spelled two ways — once doubled.
func TestVocabApply(t *testing.T) {
	cases := []struct{ label, want string }{
		{"email is valid", "when email is valid"},
		{"when in doubt", "when in doubt"},
		{"when_in_doubt", "when_in_doubt"},
		{"When_in_doubt", "When_in_doubt"},
		{"when", "when"},
		// "whenever" is a connective in its own right, not a doubled "when".
		{"whenever it rains", "whenever it rains"},
		// A label that already opens with a connective of its own reads as a
		// clause without help; "when with an empty cache" is not English.
		{"with an empty cache", "with an empty cache"},
		{"without a token", "without a token"},
		{"given an admin", "given an admin"},
		{"if the cache is cold", "if the cache is cold"},
		{"unless the flag is set", "unless the flag is set"},
		{"after a restart", "after a restart"},
		{"before the first request", "before the first request"},
		{"while the lock is held", "while the lock is held"},
		{"once the cache is warm", "once the cache is warm"},
		{"on a second call", "on a second call"},
		{"upon retry", "upon retry"},
		{"as an admin", "as an admin"},
		{"for an unknown key", "for an unknown key"},
		{"during shutdown", "during shutdown"},
		{"under load", "under load"},
		{"Given_an_admin", "Given_an_admin"},
		// Only the whole word counts: "withdrawing" does not open with "with".
		{"withdrawing funds", "when withdrawing funds"},
		{"asking twice", "when asking twice"},
	}
	for _, tc := range cases {
		if got := VocabWhen.Apply(tc.label); got != tc.want {
			t.Errorf("Apply(%q) = %q, want %q", tc.label, got, tc.want)
		}
	}
	if got := VocabNone.Apply("email is valid"); got != "email is valid" {
		t.Errorf("VocabNone.Apply = %q, want the label unchanged", got)
	}
}

// Vocabulary is display metadata. It must never reach a subtest name, or every
// -run filter, snapshot key and stored baseline in the wild would break.
func TestBuildTree_VocabularyLeavesNamesAlone(t *testing.T) {
	node := declTree(t, WithDeclarations(declsOf("example.com/pkg", map[string]Declaration{
		"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys", Vocab: VocabWhen},
	})))

	if node.Name != "returns_snake_case_keys" {
		t.Errorf("name = %q, want %q", node.Name, "returns_snake_case_keys")
	}
}
