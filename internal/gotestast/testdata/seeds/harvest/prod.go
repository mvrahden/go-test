// Package harvest is fixture test source for internal/gotestast's
// HarvestSeeds tests — loaded via golang.org/x/tools/go/packages with
// Tests: true, not compiled as part of the module's normal test/build
// graph. This file is PRODUCTION source (not a _test.go file): HarvestSeeds
// must never harvest literal call-sites from here, even though
// golang.org/x/tools/go/packages bundles production and test files into the
// same *packages.Package.Syntax for the internal test binary variant.
package harvest

// Parse is the "function under test" for ParseTestSuite: its fuzz callback
// invokes it directly, and a table test + a plain literal call elsewhere in
// the package's TEST sources exercise it with literal string inputs.
func Parse(s string) int { return len(s) }

// Echo is generic so a single callee identity can be shared by two fuzz
// callbacks with different instantiated parameter types — used to exercise
// the type-mismatch skip path with fully compiling source.
func Echo[T any](v T) T { return v }

// computedInput is not a constant literal — a table row that forwards this
// must be skipped by the harvester.
var computedInput = "computed-at-runtime"

// Msg is the struct parameter of HandleMsg — the struct-typed fuzz target
// used to pin the harvester's native-only invariant.
type Msg struct{ Text string }

// HandleMsg is the callee shared by MsgTestSuite's struct-typed fuzz
// callback and its composite-literal table test.
func HandleMsg(m Msg) int { return len(m.Text) }

// callParseFromProduction calls Parse with a literal argument from
// PRODUCTION code, inside a real function body (harvesting only inspects
// *ast.FuncDecl bodies, so the literal has to live inside one to actually
// exercise the _test.go filter). A regression test asserts this literal is
// never harvested — only _test.go call-sites qualify.
func callParseFromProduction() int {
	return Parse("from production code — must not be harvested")
}

var prodLiteralCall = callParseFromProduction()
