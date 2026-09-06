package gotestspec_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DeclarationsTestSuite covers how the tree prefers the label source declares
// over one reconstructed from the subtest name. The description contains an
// underscore, so its subtest name cannot be turned back into it: go test wrote
// underscores for the spaces, and nothing in the name says which ones were
// already there.
type DeclarationsTestSuite struct{}

func (s *DeclarationsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type declarationsCtx struct{}

func (s *DeclarationsTestSuite) BeforeEach(t *gotest.T) *declarationsCtx { return &declarationsCtx{} }

const declStream = `{"Action":"run","Package":"example.com/pkg","Test":"TestKeysTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestKeysTestSuite/TestEncode"}
{"Action":"run","Package":"example.com/pkg","Test":"TestKeysTestSuite/TestEncode/returns_snake_case_keys"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestKeysTestSuite/TestEncode/returns_snake_case_keys","Elapsed":0.001}
{"Action":"pass","Package":"example.com/pkg","Test":"TestKeysTestSuite/TestEncode","Elapsed":0.002}
{"Action":"pass","Package":"example.com/pkg","Test":"TestKeysTestSuite","Elapsed":0.003}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.004}
`

func declTree(t *gotest.T, opts ...gotestspec.BuildOption) *gotestspec.Node {
	events, err := gotestspec.ParseEvents(strings.NewReader(declStream))
	gotest.NoError(t, err)
	pkgs := gotestspec.BuildTree(events, opts...)
	gotest.Len(t, pkgs, 1)
	return pkgs[0].Nodes[0].Children[0].Children[0]
}

func declsOf(pkg string, entries map[string]gotestspec.Declaration) gotestspec.DeclarationIndex {
	return gotestspec.DeclarationIndex{pkg: entries}
}

var snakeCaseDecl = map[string]gotestspec.Declaration{
	"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys"},
}

func (s *DeclarationsTestSuite) TestBuildTree_ShowsTheDeclaredLabel(t *gotest.T, _ *declarationsCtx) {
	node := declTree(t, gotestspec.WithDeclarations(declsOf("example.com/pkg", snakeCaseDecl)))
	gotest.Equal(t, "returns snake_case keys", node.Display)
}

// Without a declaration there is nothing to prefer, and reconstructing from the
// name is the honest best effort: the rendering every stream got before.
func (s *DeclarationsTestSuite) TestBuildTree_FallsBackToTheReconstructedLabel(t *gotest.T, _ *declarationsCtx) {
	node := declTree(t)
	gotest.Equal(t, "returns snake case keys", node.Display)
}

// The index is keyed per package: a path that matches in one package must not
// hand its label to an identically-named path in another.
func (s *DeclarationsTestSuite) TestBuildTree_DeclarationsAreScopedToTheirPackage(t *gotest.T, _ *declarationsCtx) {
	node := declTree(t, gotestspec.WithDeclarations(declsOf("example.com/other", snakeCaseDecl)))
	gotest.Equal(t, "returns snake case keys", node.Display)
}

// The label is what a human reads. The name is what -run filters, snapshot keys
// and saved baselines are keyed by, and it must not move.
func (s *DeclarationsTestSuite) TestBuildTree_DeclarationsLeaveNamesAlone(t *gotest.T, _ *declarationsCtx) {
	node := declTree(t, gotestspec.WithDeclarations(declsOf("example.com/pkg", snakeCaseDecl)))
	gotest.Equal(t, "returns_snake_case_keys", node.Name)
}

func (s *DeclarationsTestSuite) TestBuildTree_SpeaksTheDeclaredVocabulary(t *gotest.T, _ *declarationsCtx) {
	node := declTree(t, gotestspec.WithDeclarations(declsOf("example.com/pkg", map[string]gotestspec.Declaration{
		"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys", Vocab: gotestspec.VocabWhen},
	})))

	gotest.Equal(t, "when returns snake_case keys", node.Display)
	gotest.Equal(t, gotestspec.VocabWhen, node.Vocab)
}

// The glyph already plays the role of "it", so an expectation stands on its own.
func (s *DeclarationsTestSuite) TestBuildTree_ExpectationsTakeNoConnective(t *gotest.T, _ *declarationsCtx) {
	node := declTree(t, gotestspec.WithDeclarations(declsOf("example.com/pkg", map[string]gotestspec.Declaration{
		"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys", Vocab: gotestspec.VocabIt},
	})))

	gotest.Equal(t, "returns snake_case keys", node.Display)
}

// The doubling guard sees the same label as source text and as a subtest name,
// where every space is an underscore. Judging those differently is how one
// behavior gets spelled two ways, once doubled.
func (s *DeclarationsTestSuite) TestVocabApply(t *gotest.T, _ *declarationsCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		want string
	}{
		{Desc: "email is valid", want: "when email is valid"},
		{Desc: "when in doubt", want: "when in doubt"},
		{Desc: "when_in_doubt", want: "when_in_doubt"},
		{Desc: "When_in_doubt", want: "When_in_doubt"},
		{Desc: "when", want: "when"},
		// "whenever" is a connective in its own right, not a doubled "when".
		{Desc: "whenever it rains", want: "whenever it rains"},
		// A label that already opens with a connective of its own reads as a
		// clause without help; "when with an empty cache" is not English.
		{Desc: "with an empty cache", want: "with an empty cache"},
		{Desc: "without a token", want: "without a token"},
		{Desc: "given an admin", want: "given an admin"},
		{Desc: "if the cache is cold", want: "if the cache is cold"},
		{Desc: "unless the flag is set", want: "unless the flag is set"},
		{Desc: "after a restart", want: "after a restart"},
		{Desc: "before the first request", want: "before the first request"},
		{Desc: "while the lock is held", want: "while the lock is held"},
		{Desc: "once the cache is warm", want: "once the cache is warm"},
		{Desc: "on a second call", want: "on a second call"},
		{Desc: "upon retry", want: "upon retry"},
		{Desc: "as an admin", want: "as an admin"},
		{Desc: "for an unknown key", want: "for an unknown key"},
		{Desc: "during shutdown", want: "during shutdown"},
		{Desc: "under load", want: "under load"},
		{Desc: "Given_an_admin", want: "Given_an_admin"},
		// Only the whole word counts: "withdrawing" does not open with "with".
		{Desc: "withdrawing funds", want: "when withdrawing funds"},
		{Desc: "asking twice", want: "when asking twice"},
	}) {
		gotest.Equal(sub, tc.want, gotestspec.VocabWhen.Apply(tc.Desc))
	}
	gotest.Equal(t, "email is valid", gotestspec.VocabNone.Apply("email is valid"), "VocabNone leaves the label unchanged")
}

// Vocabulary is display metadata. It must never reach a subtest name, or every
// -run filter, snapshot key and stored baseline in the wild would break.
func (s *DeclarationsTestSuite) TestBuildTree_VocabularyLeavesNamesAlone(t *gotest.T, _ *declarationsCtx) {
	node := declTree(t, gotestspec.WithDeclarations(declsOf("example.com/pkg", map[string]gotestspec.Declaration{
		"TestKeysTestSuite/TestEncode/returns_snake_case_keys": {Label: "returns snake_case keys", Vocab: gotestspec.VocabWhen},
	})))

	gotest.Equal(t, "returns_snake_case_keys", node.Name)
}
