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
