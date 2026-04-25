package main

import (
	"testing"

	"github.com/stretchr/testify/require"
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
		{desc: "only own args", inArgs: []string{"-ƒƒ.internal.debug"}, expectOwn: []string{"-ƒƒ.internal.debug"}, expectGoTest: nil},
		{desc: "mixed args", inArgs: []string{"-ƒƒ.internal.debug", "-v", "./...", "-race"}, expectOwn: []string{"-ƒƒ.internal.debug"}, expectGoTest: []string{"-v", "./...", "-race"}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			own, goTest := SplitArgs(tc.inArgs)
			require.Equal(t, tc.expectOwn, own)
			require.Equal(t, tc.expectGoTest, goTest)
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
		{desc: "bare relative path", args: []string{"-v", "./cmd/testsuite"}, expected: []string{"./cmd/testsuite"}},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			result := ExtractPackagePatterns(tc.args)
			require.Equal(t, tc.expected, result)
		})
	}
}
