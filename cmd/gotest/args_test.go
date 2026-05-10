package main

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestSplitArgs(t *testing.T) {
	for _, tc := range []struct {
		desc         string
		inArgs       []string
		expectOwn    []string
		expectGoTest []string
	}{
		{desc: "empty", inArgs: nil, expectOwn: nil, expectGoTest: nil},
		{desc: "only go test args", inArgs: []string{"-v", "./...", "-race", "-count=1"}, expectOwn: nil, expectGoTest: []string{"-v", "./...", "-race", "-count=1"}},
		{desc: "only own args", inArgs: []string{"--debug"}, expectOwn: []string{"--debug"}, expectGoTest: nil},
		{desc: "mixed args", inArgs: []string{"--debug", "-v", "./...", "-race"}, expectOwn: []string{"--debug"}, expectGoTest: []string{"-v", "./...", "-race"}},
		{desc: "min flag with equals", inArgs: []string{"--min=80", "-v"}, expectOwn: []string{"--min=80"}, expectGoTest: []string{"-v"}},
		{desc: "min flag with space", inArgs: []string{"--min", "90", "-v"}, expectOwn: []string{"--min", "90"}, expectGoTest: []string{"-v"}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			own, goTest := SplitArgs(tc.inArgs)
			gotest.Equal(t, tc.expectOwn, own)
			gotest.Equal(t, tc.expectGoTest, goTest)
		})
	}
}

func TestParseSubcommand(t *testing.T) {
	for _, tc := range []struct {
		desc              string
		args              []string
		expectSubcmd      string
		expectRemaining   []string
	}{
		{desc: "empty args", args: nil, expectSubcmd: "", expectRemaining: nil},
		{desc: "no subcommand, just flags", args: []string{"-v", "./..."}, expectSubcmd: "", expectRemaining: []string{"-v", "./..."}},
		{desc: "version subcommand", args: []string{"version"}, expectSubcmd: "version", expectRemaining: nil},
		{desc: "scaffold subcommand", args: []string{"scaffold", "-v"}, expectSubcmd: "scaffold", expectRemaining: []string{"-v"}},
		{desc: "migrate subcommand", args: []string{"migrate"}, expectSubcmd: "migrate", expectRemaining: nil},
		{desc: "help subcommand", args: []string{"help"}, expectSubcmd: "help", expectRemaining: nil},
		{desc: "generate subcommand", args: []string{"generate", "./..."}, expectSubcmd: "generate", expectRemaining: []string{"./..."}},
		{desc: "watch subcommand", args: []string{"watch"}, expectSubcmd: "watch", expectRemaining: nil},
{desc: "clean subcommand", args: []string{"clean", "./..."}, expectSubcmd: "clean", expectRemaining: []string{"./..."}},
		{desc: "spec subcommand", args: []string{"spec"}, expectSubcmd: "spec", expectRemaining: nil},
		{desc: "unknown first arg is not consumed", args: []string{"./...", "-v"}, expectSubcmd: "", expectRemaining: []string{"./...", "-v"}},
		{desc: "flag first arg is not consumed", args: []string{"-v", "./..."}, expectSubcmd: "", expectRemaining: []string{"-v", "./..."}},
		{desc: "package pattern not consumed", args: []string{"github.com/foo/bar"}, expectSubcmd: "", expectRemaining: []string{"github.com/foo/bar"}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			subcmd, remaining := ParseSubcommand(tc.args)
			gotest.Equal(t, tc.expectSubcmd, subcmd)
			gotest.Equal(t, tc.expectRemaining, remaining)
		})
	}
}

func TestExtractPackagePatterns(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		args     []string
		expected []string
	}{
		{desc: "explicit relative path", args: []string{"-v", "./...", "-race"}, expected: []string{"./..."}},
		{desc: "explicit named package", args: []string{"-v", "github.com/foo/bar", "-race"}, expected: []string{"github.com/foo/bar"}},
		{desc: "no package defaults to dot", args: []string{"-v", "-race"}, expected: []string{"."}},
		{desc: "multiple packages", args: []string{"./pkg/a", "./pkg/b", "-v"}, expected: []string{"./pkg/a", "./pkg/b"}},
		{desc: "stops at -args", args: []string{"-v", "./...", "-args", "-custom", "./not/a/pkg"}, expected: []string{"./..."}},
		{desc: "no args defaults to dot", args: nil, expected: []string{"."}},
		{desc: "bare relative path", args: []string{"-v", "./cmd/gotest"}, expected: []string{"./cmd/gotest"}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			result := ExtractPackagePatterns(tc.args)
			gotest.Equal(t, tc.expected, result)
		})
	}
}

func TestLooksLikePackagePattern(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		input  string
		expect bool
	}{
		{desc: "relative path", input: "./pkg/foo", expect: true},
		{desc: "absolute path", input: "/usr/local/pkg", expect: true},
		{desc: "named package", input: "github.com/foo/bar", expect: true},
		{desc: "flag", input: "-v", expect: false},
		{desc: "bare word", input: "strings", expect: false},
		{desc: "dot only", input: ".", expect: true},
		{desc: "dot-slash", input: "./...", expect: true},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotest.Equal(t, tc.expect, looksLikePackagePattern(tc.input))
		})
	}
}

