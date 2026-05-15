package gotestrunner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

// --- reference implementations (pre-refactor logic) ---

// buildPlainArgs reproduces the exact arg logic of the old RunSingleSuite.
func buildPlainArgs(target SuiteTarget) (path string, args []string) {
	if target.RunFilter != "" {
		args = append(args, "-test.run="+target.RunFilter)
	} else {
		args = append(args, fmt.Sprintf("-test.run=^%s$", regexp.QuoteMeta(target.SuiteName)))
	}
	args = append(args, target.RunFlags...)
	if target.CoverProfile != "" {
		args = append(args, "-test.coverprofile="+target.CoverProfile)
	}
	return target.BinaryPath, args
}

// buildTest2JSONArgs reproduces the exact arg logic of the old RunSingleSuiteTest2JSON.
func buildTest2JSONArgs(target SuiteTarget) (path string, args []string) {
	var testArgs []string
	if target.RunFilter != "" {
		testArgs = append(testArgs, "-test.run="+target.RunFilter)
	} else {
		testArgs = append(testArgs, fmt.Sprintf("-test.run=^%s$", regexp.QuoteMeta(target.SuiteName)))
	}
	testArgs = append(testArgs, "-test.v=test2json")
	for _, f := range target.RunFlags {
		if f == "-test.v" || strings.HasPrefix(f, "-test.v=") {
			continue
		}
		testArgs = append(testArgs, f)
	}
	if target.CoverProfile != "" {
		testArgs = append(testArgs, "-test.coverprofile="+target.CoverProfile)
	}
	args = []string{"tool", "test2json", "-p", target.Package, "-t", target.BinaryPath}
	args = append(args, testArgs...)
	return "go", args
}

// --- buildSuiteCmd tests ---

func TestBuildSuiteCmd(t *testing.T) {
	ctx := context.Background()
	env := []string{"PATH=/usr/bin", "HOME=/home/test"}

	for _, tc := range []struct {
		name       string
		target     SuiteTarget
		test2json  bool
		wantBinary string
		wantArgs   []string
	}{
		{
			name: "plain/basic suite",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFooSuite",
			},
			test2json:  false,
			wantBinary: "/tmp/pkg.test",
			wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$"},
		},
		{
			name: "plain/run filter overrides suite name",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFooSuite",
				RunFilter:  "^TestFooSuite$/^TestBar$",
			},
			test2json:  false,
			wantBinary: "/tmp/pkg.test",
			wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$/^TestBar$"},
		},
		{
			name: "plain/with run flags",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFooSuite",
				RunFlags:   []string{"-test.timeout=30s", "-test.count=1"},
			},
			test2json:  false,
			wantBinary: "/tmp/pkg.test",
			wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$", "-test.timeout=30s", "-test.count=1"},
		},
		{
			name: "plain/keeps -test.v in run flags",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFooSuite",
				RunFlags:   []string{"-test.v", "-test.timeout=10s"},
			},
			test2json:  false,
			wantBinary: "/tmp/pkg.test",
			wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$", "-test.v", "-test.timeout=10s"},
		},
		{
			name: "plain/with cover profile",
			target: SuiteTarget{
				Package:      "example.com/pkg",
				BinaryPath:   "/tmp/pkg.test",
				SuiteName:    "TestFooSuite",
				CoverProfile: "/tmp/cover.out",
			},
			test2json:  false,
			wantBinary: "/tmp/pkg.test",
			wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$", "-test.coverprofile=/tmp/cover.out"},
		},
		{
			name: "plain/suite name with regex-special chars",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFoo.Bar+Baz",
			},
			test2json:  false,
			wantBinary: "/tmp/pkg.test",
			wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFoo\\.Bar\\+Baz$"},
		},
		{
			name: "plain/all fields populated",
			target: SuiteTarget{
				Package:      "example.com/pkg",
				BinaryPath:   "/tmp/pkg.test",
				SuiteName:    "TestFooSuite",
				RunFilter:    "^TestFooSuite$/^TestBar$",
				RunFlags:     []string{"-test.timeout=30s", "-test.v"},
				CoverProfile: "/tmp/cover.out",
			},
			test2json:  false,
			wantBinary: "/tmp/pkg.test",
			wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$/^TestBar$", "-test.timeout=30s", "-test.v", "-test.coverprofile=/tmp/cover.out"},
		},
		{
			name: "json/basic suite",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFooSuite",
			},
			test2json:  true,
			wantBinary: "go",
			wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
				"-test.run=^TestFooSuite$", "-test.v=test2json"},
		},
		{
			name: "json/run filter overrides suite name",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFooSuite",
				RunFilter:  "^TestFooSuite$/^TestBar$",
			},
			test2json:  true,
			wantBinary: "go",
			wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
				"-test.run=^TestFooSuite$/^TestBar$", "-test.v=test2json"},
		},
		{
			name: "json/strips -test.v from run flags",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFooSuite",
				RunFlags:   []string{"-test.v", "-test.timeout=30s"},
			},
			test2json:  true,
			wantBinary: "go",
			wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
				"-test.run=^TestFooSuite$", "-test.v=test2json", "-test.timeout=30s"},
		},
		{
			name: "json/strips -test.v=true from run flags",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFooSuite",
				RunFlags:   []string{"-test.v=true"},
			},
			test2json:  true,
			wantBinary: "go",
			wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
				"-test.run=^TestFooSuite$", "-test.v=test2json"},
		},
		{
			name: "json/with cover profile",
			target: SuiteTarget{
				Package:      "example.com/pkg",
				BinaryPath:   "/tmp/pkg.test",
				SuiteName:    "TestFooSuite",
				CoverProfile: "/tmp/cover.out",
			},
			test2json:  true,
			wantBinary: "go",
			wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
				"-test.run=^TestFooSuite$", "-test.v=test2json", "-test.coverprofile=/tmp/cover.out"},
		},
		{
			name: "json/all fields, -test.v stripped",
			target: SuiteTarget{
				Package:      "example.com/pkg",
				BinaryPath:   "/tmp/pkg.test",
				SuiteName:    "TestFooSuite",
				RunFilter:    "^TestFooSuite$/^TestBar$",
				RunFlags:     []string{"-test.v", "-test.timeout=30s", "-test.count=1"},
				CoverProfile: "/tmp/cover.out",
			},
			test2json:  true,
			wantBinary: "go",
			wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
				"-test.run=^TestFooSuite$/^TestBar$", "-test.v=test2json",
				"-test.timeout=30s", "-test.count=1",
				"-test.coverprofile=/tmp/cover.out"},
		},
		{
			name: "json/suite name with regex-special chars",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFoo.Bar+Baz",
			},
			test2json:  true,
			wantBinary: "go",
			wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
				"-test.run=^TestFoo\\.Bar\\+Baz$", "-test.v=test2json"},
		},
		{
			name: "json/standalone group",
			target: SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "(standalone)",
				RunFilter:  "^(TestAlpha|TestBeta)$",
			},
			test2json:  true,
			wantBinary: "go",
			wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
				"-test.run=^(TestAlpha|TestBeta)$", "-test.v=test2json"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := buildSuiteCmd(ctx, tc.target, env, tc.test2json)

			// For "go", cmd.Path is resolved to the absolute path; compare Args[0] loosely.
			if tc.wantBinary == "go" {
				if !strings.HasSuffix(cmd.Path, "/go") && cmd.Path != "go" {
					t.Errorf("binary: got %q, want suffix /go", cmd.Path)
				}
			} else if cmd.Path != tc.wantBinary {
				t.Errorf("binary: got %q, want %q", cmd.Path, tc.wantBinary)
			}

			// Compare full args list. For the "go" binary, Args[0] is resolved.
			if len(cmd.Args) != len(tc.wantArgs) {
				t.Fatalf("args length: got %d %v, want %d %v", len(cmd.Args), cmd.Args, len(tc.wantArgs), tc.wantArgs)
			}
			for i := range cmd.Args {
				if i == 0 && tc.wantBinary == "go" {
					if !strings.HasSuffix(cmd.Args[0], "/go") && cmd.Args[0] != "go" {
						t.Errorf("args[0]: got %q, want suffix /go", cmd.Args[0])
					}
					continue
				}
				if cmd.Args[i] != tc.wantArgs[i] {
					t.Errorf("args[%d]: got %q, want %q", i, cmd.Args[i], tc.wantArgs[i])
				}
			}

			if len(cmd.Env) != len(env) {
				t.Errorf("env length: got %d, want %d", len(cmd.Env), len(env))
			}
		})
	}
}

// --- Cross-check: buildSuiteCmd matches pre-refactor reference implementations ---

func TestBuildSuiteCmd_MatchesOriginalPlain(t *testing.T) {
	targets := []SuiteTarget{
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX"},
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$"},
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFlags: []string{"-test.v", "-test.timeout=5s"}},
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", CoverProfile: "/c.out"},
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$", RunFlags: []string{"-test.count=2"}, CoverProfile: "/c.out"},
	}
	ctx := context.Background()
	env := []string{"A=1"}

	for _, target := range targets {
		t.Run(target.SuiteName+"_"+target.RunFilter, func(t *testing.T) {
			refPath, refArgs := buildPlainArgs(target)
			cmd := buildSuiteCmd(ctx, target, env, false)

			if cmd.Args[0] != refPath {
				t.Errorf("path: got %q, want %q", cmd.Args[0], refPath)
			}
			gotArgs := cmd.Args[1:]
			if len(gotArgs) != len(refArgs) {
				t.Fatalf("args: got %v, want %v", gotArgs, refArgs)
			}
			for i := range gotArgs {
				if gotArgs[i] != refArgs[i] {
					t.Errorf("args[%d]: got %q, want %q", i, gotArgs[i], refArgs[i])
				}
			}
		})
	}
}

func TestBuildSuiteCmd_MatchesOriginalTest2JSON(t *testing.T) {
	targets := []SuiteTarget{
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX"},
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$"},
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFlags: []string{"-test.v", "-test.timeout=5s"}},
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFlags: []string{"-test.v=true"}},
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", CoverProfile: "/c.out"},
		{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$", RunFlags: []string{"-test.v", "-test.count=2"}, CoverProfile: "/c.out"},
	}
	ctx := context.Background()
	env := []string{"A=1"}

	for _, target := range targets {
		t.Run(target.SuiteName+"_"+target.RunFilter, func(t *testing.T) {
			_, refArgs := buildTest2JSONArgs(target)
			cmd := buildSuiteCmd(ctx, target, env, true)

			// cmd.Args[0] is resolved "go" path; refArgs starts at "tool".
			// Compare cmd.Args[1:] against refArgs (which doesn't include "go").
			gotArgs := cmd.Args[1:]
			if len(gotArgs) != len(refArgs) {
				t.Fatalf("args: got %v, want %v", gotArgs, refArgs)
			}
			for i := range gotArgs {
				if gotArgs[i] != refArgs[i] {
					t.Errorf("args[%d]: got %q, want %q", i, gotArgs[i], refArgs[i])
				}
			}
		})
	}
}

func TestBuildSuiteCmd_Test2JSON_ResolvesGoBinary(t *testing.T) {
	ctx := context.Background()
	target := SuiteTarget{
		Package:    "example.com/pkg",
		BinaryPath: "/tmp/pkg.test",
		SuiteName:  "TestFoo",
	}
	cmd := buildSuiteCmd(ctx, target, nil, true)

	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not in PATH")
	}
	if cmd.Path != goPath {
		t.Errorf("cmd.Path = %q, want %q", cmd.Path, goPath)
	}
}

// --- PackageBatcher tests ---

func TestPackageBatcher_Record(t *testing.T) {
	b := NewPackageBatcher()
	b.Register("pkg/a", 3)
	b.Register("pkg/b", 1)

	r := SuiteResult{ExitCode: 0}

	if b.Record("pkg/a", 0, r) {
		t.Error("expected false after 1 of 3")
	}
	if b.Record("pkg/a", 2, r) {
		t.Error("expected false after 2 of 3")
	}
	if !b.Record("pkg/a", 1, r) {
		t.Error("expected true after 3 of 3")
	}

	if !b.Record("pkg/b", 0, r) {
		t.Error("expected true after 1 of 1")
	}
}

func TestPackageBatcher_Flush(t *testing.T) {
	b := NewPackageBatcher()
	b.Register("example.com/pkg", 2)

	b.Record("example.com/pkg", 0, SuiteResult{
		Stdout:   []byte("=== RUN   TestA\n--- PASS: TestA (0.00s)\nPASS\n"),
		ExitCode: 0,
		Duration: 100 * time.Millisecond,
	})
	b.Record("example.com/pkg", 1, SuiteResult{
		Stdout:   []byte("=== RUN   TestB\n--- FAIL: TestB (0.00s)\nFAIL\n"),
		Stderr:   []byte("some error\n"),
		ExitCode: 1,
		Duration: 200 * time.Millisecond,
	})

	// Capture stdout and stderr.
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	b.Flush("example.com/pkg")

	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	var bufOut, bufErr bytes.Buffer
	bufOut.ReadFrom(rOut)
	bufErr.ReadFrom(rErr)
	rOut.Close()
	rErr.Close()

	wantOut := "=== RUN   TestA\n--- PASS: TestA (0.00s)\n" +
		"=== RUN   TestB\n--- FAIL: TestB (0.00s)\n" +
		"FAIL\nFAIL\texample.com/pkg\t0.300s\n"
	if bufOut.String() != wantOut {
		t.Errorf("stdout:\ngot:  %q\nwant: %q", bufOut.String(), wantOut)
	}
	if bufErr.String() != "some error\n" {
		t.Errorf("stderr: got %q, want %q", bufErr.String(), "some error\n")
	}
}

func TestPackageBatcher_Flush_AllPassing(t *testing.T) {
	b := NewPackageBatcher()
	b.Register("example.com/ok", 1)
	b.Record("example.com/ok", 0, SuiteResult{
		Stdout:   []byte("=== RUN   TestOK\n--- PASS: TestOK (0.00s)\nPASS\n"),
		ExitCode: 0,
		Duration: 50 * time.Millisecond,
	})

	oldOut := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	b.Flush("example.com/ok")

	wOut.Close()
	os.Stdout = oldOut
	var buf bytes.Buffer
	buf.ReadFrom(rOut)
	rOut.Close()

	wantOut := "=== RUN   TestOK\n--- PASS: TestOK (0.00s)\n" +
		"PASS\nok  \texample.com/ok\t0.050s\n"
	if buf.String() != wantOut {
		t.Errorf("stdout:\ngot:  %q\nwant: %q", buf.String(), wantOut)
	}
}

// --- StripTrailingStatus tests ---

func TestStripTrailingStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "strips trailing PASS",
			input:  "=== RUN   TestFoo\n--- PASS: TestFoo (0.00s)\nPASS\n",
			expect: "=== RUN   TestFoo\n--- PASS: TestFoo (0.00s)\n",
		},
		{
			name:   "strips trailing FAIL",
			input:  "=== RUN   TestFoo\n--- FAIL: TestFoo (0.00s)\nFAIL\n",
			expect: "=== RUN   TestFoo\n--- FAIL: TestFoo (0.00s)\n",
		},
		{
			name:   "strips trailing PASS with extra newlines",
			input:  "line1\nline2\nPASS\n\n\n",
			expect: "line1\nline2\n",
		},
		{
			name:   "preserves non-status last line",
			input:  "line1\nline2\nsome output\n",
			expect: "line1\nline2\nsome output\n",
		},
		{
			name:   "only PASS returns nil",
			input:  "PASS\n",
			expect: "",
		},
		{
			name:   "no newlines returns nil",
			input:  "PASS",
			expect: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := StripTrailingStatus([]byte(tc.input))
			if tc.expect == "" {
				if got != nil {
					t.Errorf("expected nil, got %q", got)
				}
			} else if string(got) != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}

// --- WritePackageSummary tests ---

func TestWritePackageSummary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pkg      string
		failed   bool
		duration time.Duration
		expect   string
	}{
		{
			name:     "passing package",
			pkg:      "example.com/pkg",
			failed:   false,
			duration: 1234 * time.Millisecond,
			expect:   "PASS\nok  \texample.com/pkg\t1.234s\n",
		},
		{
			name:     "failing package",
			pkg:      "example.com/pkg",
			failed:   true,
			duration: 567 * time.Millisecond,
			expect:   "FAIL\nFAIL\texample.com/pkg\t0.567s\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, w, _ := os.Pipe()
			old := os.Stdout
			os.Stdout = w
			WritePackageSummary(tc.pkg, tc.failed, tc.duration)
			w.Close()
			os.Stdout = old
			var buf bytes.Buffer
			buf.ReadFrom(r)
			r.Close()
			if buf.String() != tc.expect {
				t.Errorf("got %q, want %q", buf.String(), tc.expect)
			}
		})
	}
}

// --- existing tests ---

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
