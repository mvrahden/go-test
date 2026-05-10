package gotestrunner

import (
	"testing"
)

func TestSplitTopLevelOr(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  string
		expect []string
	}{
		{"no pipe", `^TestFoo$`, []string{`^TestFoo$`}},
		{"two alternatives", `^TestA$|^TestB$`, []string{`^TestA$`, `^TestB$`}},
		{"pipe inside parens", `^Test$/^(A|B)$`, []string{`^Test$/^(A|B)$`}},
		{"pipe inside brackets", `^Test[a|b]$`, []string{`^Test[a|b]$`}},
		{"mixed top and nested", `^TestA$/^(X|Y)$|^TestB$/^Z$`, []string{`^TestA$/^(X|Y)$`, `^TestB$/^Z$`}},
		{"escaped pipe", `^Test\|Foo$`, []string{`^Test\|Foo$`}},
		{"nested parens", `^Test$/^((A|B)|C)$`, []string{`^Test$/^((A|B)|C)$`}},
		{"three alternatives", `^A$|^B$|^C$`, []string{`^A$`, `^B$`, `^C$`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := splitTopLevelOr(tc.input)
			if len(got) != len(tc.expect) {
				t.Fatalf("got %d parts %v, want %d parts %v", len(got), got, len(tc.expect), tc.expect)
			}
			for i := range got {
				if got[i] != tc.expect[i] {
					t.Errorf("part[%d]: got %q, want %q", i, got[i], tc.expect[i])
				}
			}
		})
	}
}

func TestSuiteRunFilter(t *testing.T) {
	for _, tc := range []struct {
		name         string
		userFilter   string
		testFuncName string
		expect       string
	}{
		{
			name:         "empty filter",
			userFilter:   "",
			testFuncName: "TestFooSuite",
			expect:       "",
		},
		{
			name:         "suite only (no subtest)",
			userFilter:   "^TestFooSuite$",
			testFuncName: "TestFooSuite",
			expect:       "",
		},
		{
			name:         "single method filter",
			userFilter:   "^TestFooSuite$/^TestBar$",
			testFuncName: "TestFooSuite",
			expect:       "^TestFooSuite$/^TestBar$",
		},
		{
			name:         "multi-method same suite",
			userFilter:   "^TestFooSuite$/^(TestBar|TestBaz)$",
			testFuncName: "TestFooSuite",
			expect:       "^TestFooSuite$/^(TestBar|TestBaz)$",
		},
		{
			name:         "multi-suite picks matching",
			userFilter:   "^TestSuiteA$/^TestX$|^TestSuiteB$/^TestY$",
			testFuncName: "TestSuiteA",
			expect:       "^TestSuiteA$/^TestX$",
		},
		{
			name:         "multi-suite picks other",
			userFilter:   "^TestSuiteA$/^TestX$|^TestSuiteB$/^TestY$",
			testFuncName: "TestSuiteB",
			expect:       "^TestSuiteB$/^TestY$",
		},
		{
			name:         "multi-suite no match",
			userFilter:   "^TestSuiteA$/^TestX$|^TestSuiteB$/^TestY$",
			testFuncName: "TestSuiteC",
			expect:       "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := suiteRunFilter(tc.userFilter, tc.testFuncName)
			if got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}
