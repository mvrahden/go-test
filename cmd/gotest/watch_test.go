package main

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestIsGoFile(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		name   string
		expect bool
	}{
		{desc: "go file", name: "main.go", expect: true},
		{desc: "test file", name: "main_test.go", expect: true},
		{desc: "path with go file", name: "/tmp/foo/bar.go", expect: true},
		{desc: "not a go file", name: "main.py", expect: false},
		{desc: "go in middle", name: "foo.go.bak", expect: false},
		{desc: "empty", name: "", expect: false},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			gotest.Equal(t, tc.expect, isGoFile(tc.name))
		})
	}
}

func TestDirsToPatterns(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		dirs    map[string]bool
		lenWant int
	}{
		{desc: "single dir", dirs: map[string]bool{"pkg/foo": true}, lenWant: 1},
		{desc: "multiple dirs", dirs: map[string]bool{"pkg/foo": true, "cmd/bar": true}, lenWant: 2},
		{desc: "empty", dirs: map[string]bool{}, lenWant: 0},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			result := dirsToPatterns(tc.dirs)
			gotest.Len(t, result, tc.lenWant)
			for _, p := range result {
				gotest.True(t, len(p) > 2 && p[:2] == "./", "expected ./ prefix, got: %s", p)
			}
		})
	}
}

func TestReplacePatterns(t *testing.T) {
	for _, tc := range []struct {
		desc        string
		original    []string
		newPatterns []string
		expected    []string
	}{
		{
			desc:        "replaces package pattern",
			original:    []string{"-v", "./pkg/foo", "-race"},
			newPatterns: []string{"./cmd/bar"},
			expected:    []string{"-v", "-race", "./cmd/bar"},
		},
		{
			desc:        "no patterns to replace",
			original:    []string{"-v", "-race"},
			newPatterns: []string{"./pkg/new"},
			expected:    []string{"-v", "-race", "./pkg/new"},
		},
		{
			desc:        "multiple patterns replaced",
			original:    []string{"-v", "./pkg/a", "./pkg/b", "-race"},
			newPatterns: []string{"./changed"},
			expected:    []string{"-v", "-race", "./changed"},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			result := replacePatterns(tc.original, tc.newPatterns)
			gotest.Equal(t, tc.expected, result)
		})
	}
}
