package gotestrunner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// --- reference implementations (pre-refactor logic) ---

// buildPlainArgs reproduces the exact arg logic of the old RunSingleSuite.
func buildPlainArgs(target gotestrunner.SuiteTarget) (path string, args []string) {
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
func buildTest2JSONArgs(target gotestrunner.SuiteTarget) (path string, args []string) {
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

// GotestrunnerTestSuite tests runner internals: flag classification, overlay
// management, command building, output formatting, and suite filtering.
type GotestrunnerTestSuite struct{}

// --- args tests ---

func (s *GotestrunnerTestSuite) TestIsGoTestFlag(t *gotest.T) {
	for _, tc := range []struct {
		name    string
		flag    string
		isValue bool
		known   bool
	}{
		{"build bool", "-race", false, true},
		{"build value", "-tags", true, true},
		{"build special value", "-o", true, true},
		{"run bool", "-v", false, true},
		{"run value", "-run", true, true},
		{"json", "-json", false, true},
		{"args", "-args", false, true},
		{"unknown", "-zzz", false, false},
		{"double dash unknown", "--debug", false, false},
	} {
		t.When(tc.name, func(w *gotest.T) {
			w.It("returns expected isValue and known", func(it *gotest.T) {
				isValue, known := gotestrunner.IsGoTestFlag(tc.flag)
				gotest.Equal(it, tc.isValue, isValue)
				gotest.Equal(it, tc.known, known)
			})
		})
	}
}

func (s *GotestrunnerTestSuite) TestCoverProfile(t *gotest.T) {
	t.When("extracting from flags", func(w *gotest.T) {
		for _, tc := range []struct {
			name   string
			flags  []string
			expect string
		}{
			{"empty", nil, ""},
			{"equals form", []string{"-v", "-coverprofile=cover.out"}, "cover.out"},
			{"space form", []string{"-coverprofile", "cover.out", "-v"}, "cover.out"},
			{"stops at -args", []string{"-args", "-coverprofile=cover.out"}, ""},
			{"no coverprofile", []string{"-v", "-count=1"}, ""},
		} {
			w.It(tc.name, func(it *gotest.T) {
				got := gotestrunner.ExtractCoverProfile(tc.flags)
				gotest.Equal(it, tc.expect, got)
			})
		}
	})

	t.When("stripping from flags", func(w *gotest.T) {
		for _, tc := range []struct {
			name   string
			flags  []string
			expect []string
		}{
			{"empty", nil, nil},
			{"equals form", []string{"-v", "-coverprofile=cover.out", "-count=1"}, []string{"-v", "-count=1"}},
			{"space form", []string{"-coverprofile", "cover.out", "-v"}, []string{"-v"}},
			{"preserves -args passthrough", []string{"-v", "-args", "-coverprofile=x"}, []string{"-v", "-args", "-coverprofile=x"}},
			{"no coverprofile unchanged", []string{"-v", "-count=1"}, []string{"-v", "-count=1"}},
		} {
			w.It(tc.name, func(it *gotest.T) {
				got := gotestrunner.StripCoverProfile(tc.flags)
				gotest.Equal(it, tc.expect, got)
			})
		}
	})

	t.When("merging profiles", func(w *gotest.T) {
		w.When("two profiles with overlapping blocks", func(w2 *gotest.T) {
			w2.It("merges and sorts with max-count aggregation", func(it *gotest.T) {
				dir := it.T().TempDir()

				writeProfile := func(name, content string) string {
					p := filepath.Join(dir, name)
					err := os.WriteFile(p, []byte(content), 0o644)
					gotest.NoError(it, err)
					return p
				}

				p1 := writeProfile("a.out", "mode: set\nfoo/bar.go:1.2,3.4 1 1\nfoo/bar.go:5.6,7.8 1 0\n")
				p2 := writeProfile("b.out", "mode: set\nfoo/bar.go:5.6,7.8 1 1\nfoo/baz.go:1.2,3.4 1 1\n")

				out := filepath.Join(dir, "merged.out")
				err := gotestrunner.MergeCoverProfiles([]string{p1, p2}, out)
				gotest.NoError(it, err)

				data, err := os.ReadFile(out)
				gotest.NoError(it, err)
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")

				gotest.Equal(it, "mode: set", lines[0])
				gotest.Equal(it, 4, len(lines))

				// Verify sorted order: foo/bar.go blocks before foo/baz.go
				gotest.True(it, strings.HasPrefix(lines[1], "foo/bar.go"))
				gotest.True(it, strings.HasPrefix(lines[2], "foo/bar.go"))
				gotest.True(it, strings.HasPrefix(lines[3], "foo/baz.go"))

				// Verify max-count aggregation: foo/bar.go:5.6,7.8 should be 1 (max of 0,1)
				gotest.True(it, strings.HasSuffix(lines[2], " 1"))
			})
		})

		w.When("profile A has uncovered block not in profile B", func(w2 *gotest.T) {
			w2.It("preserves uncovered blocks with count 0", func(it *gotest.T) {
				dir := it.T().TempDir()

				writeProfile := func(name, content string) string {
					p := filepath.Join(dir, name)
					err := os.WriteFile(p, []byte(content), 0o644)
					gotest.NoError(it, err)
					return p
				}

				pA := writeProfile("a.out", "mode: set\nfoo/bar.go:1.2,3.4 1 1\nfoo/bar.go:10.1,12.5 1 0\n")
				pB := writeProfile("b.out", "mode: set\nfoo/baz.go:1.2,3.4 1 1\n")

				out := filepath.Join(dir, "merged.out")
				err := gotestrunner.MergeCoverProfiles([]string{pA, pB}, out)
				gotest.NoError(it, err)

				data, err := os.ReadFile(out)
				gotest.NoError(it, err)
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")

				gotest.Equal(it, 4, len(lines))
				gotest.True(it, slices.Contains(lines, "foo/bar.go:10.1,12.5 1 0"))
			})
		})

		w.When("one profile is missing", func(w2 *gotest.T) {
			w2.It("skips the missing file", func(it *gotest.T) {
				dir := it.T().TempDir()
				p := filepath.Join(dir, "exists.out")
				os.WriteFile(p, []byte("mode: set\nfoo.go:1.2,3.4 1 1\n"), 0o644)

				out := filepath.Join(dir, "merged.out")
				err := gotestrunner.MergeCoverProfiles([]string{filepath.Join(dir, "missing.out"), p}, out)
				gotest.NoError(it, err)

				data, _ := os.ReadFile(out)
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				gotest.Equal(it, 2, len(lines))
			})
		})
	})
}

// --- overlay tests ---

func (s *GotestrunnerTestSuite) TestOverlayManagement(t *gotest.T) {
	t.When("writing overlay", func(w *gotest.T) {
		w.It("creates correct overlay entries for PTest and PXTest", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n"), PXTest: []byte("package a_test\n")},
				{AbsPath: "/fake/pkg/b", PTest: []byte("package b\n")},
			}

			tmpDir, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			defer os.RemoveAll(tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, "overlay.json"))
			gotest.NoError(it, err)
			var ov gotestrunner.ExportOverlayJSON
			err = json.Unmarshal(data, &ov)
			gotest.NoError(it, err)

			wantKeys := map[string]bool{
				filepath.Join("/fake/pkg/a", about.PSuite):  true,
				filepath.Join("/fake/pkg/a", about.PXSuite): true,
				filepath.Join("/fake/pkg/b", about.PSuite):  true,
			}
			gotest.Equal(it, len(wantKeys), len(ov.Replace))
			for virtual, real := range ov.Replace {
				gotest.True(it, wantKeys[virtual], "unexpected overlay key: %s", virtual)
				_, err := os.Stat(real)
				gotest.NoError(it, err)
			}

			bPXSuite := filepath.Join("/fake/pkg/b", about.PXSuite)
			_, ok := ov.Replace[bPXSuite]
			gotest.False(it, ok, "pkg/b should not have PXSuite mapping (empty PXTest)")
		})

		w.It("produces unique overlay directories when called twice", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n")},
			}

			dir1, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			defer os.RemoveAll(dir1)

			dir2, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			defer os.RemoveAll(dir2)

			gotest.NotEqual(it, dir1, dir2)
		})

		w.It("contains a .pid file with the current PID", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/a", PTest: []byte("package a\n")},
			}

			tmpDir, err := gotestrunner.WriteOverlay(results)
			gotest.NoError(it, err)
			defer os.RemoveAll(tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, ".pid"))
			gotest.NoError(it, err)

			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			gotest.NoError(it, err)
			gotest.Equal(it, os.Getpid(), pid)
		})

		w.It("creates an empty overlay for nil results", func(it *gotest.T) {
			tmpDir, err := gotestrunner.WriteOverlay(nil)
			gotest.NoError(it, err)
			defer os.RemoveAll(tmpDir)

			data, err := os.ReadFile(filepath.Join(tmpDir, "overlay.json"))
			gotest.NoError(it, err)
			var ov gotestrunner.ExportOverlayJSON
			err = json.Unmarshal(data, &ov)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, len(ov.Replace))
		})
	})

	t.When("cleaning stale overlays", func(w *gotest.T) {
		w.It("removes overlay with dead PID", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)
			// Write a PID that doesn't exist (use a very high PID)
			os.WriteFile(filepath.Join(dir, ".pid"), []byte("999999999"), 0644)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected stale overlay to be removed")
		})

		w.It("keeps overlay with live PID", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)
			defer os.RemoveAll(dir)

			// Write our own PID -- guaranteed alive
			os.WriteFile(filepath.Join(dir, ".pid"), []byte(strconv.Itoa(os.Getpid())), 0644)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.False(it, os.IsNotExist(err), "expected live overlay to be kept")
		})

		w.It("removes overlay with no PID file", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "gotest-overlay-test-")
			gotest.NoError(it, err)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "expected overlay without PID file to be removed")
		})

		w.It("ignores non-overlay directories", func(it *gotest.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "unrelated-")
			gotest.NoError(it, err)
			defer os.RemoveAll(dir)

			gotestrunner.CleanStaleOverlays()

			_, err = os.Stat(dir)
			gotest.False(it, os.IsNotExist(err), "expected non-overlay dir to be untouched")
		})
	})
}

// --- buildSuiteCmd tests ---

func (s *GotestrunnerTestSuite) TestBuildSuiteCmd(t *gotest.T) {
	t.When("plain mode", func(w *gotest.T) {
		ctx := context.Background()
		env := []string{"PATH=/usr/bin", "HOME=/home/test"}

		for _, tc := range []struct {
			name       string
			target     gotestrunner.SuiteTarget
			wantBinary string
			wantArgs   []string
		}{
			{
				name: "basic suite",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFooSuite",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$"},
			},
			{
				name: "run filter overrides suite name",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFooSuite",
					RunFilter:  "^TestFooSuite$/^TestBar$",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$/^TestBar$"},
			},
			{
				name: "with run flags",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFooSuite",
					RunFlags:   []string{"-test.timeout=30s", "-test.count=1"},
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$", "-test.timeout=30s", "-test.count=1"},
			},
			{
				name: "keeps -test.v in run flags",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFooSuite",
					RunFlags:   []string{"-test.v", "-test.timeout=10s"},
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$", "-test.v", "-test.timeout=10s"},
			},
			{
				name: "with cover profile",
				target: gotestrunner.SuiteTarget{
					Package:      "example.com/pkg",
					BinaryPath:   "/tmp/pkg.test",
					SuiteName:    "TestFooSuite",
					CoverProfile: "/tmp/cover.out",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$", "-test.coverprofile=/tmp/cover.out"},
			},
			{
				name: "suite name with regex-special chars",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFoo.Bar+Baz",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFoo\\.Bar\\+Baz$"},
			},
			{
				name: "all fields populated",
				target: gotestrunner.SuiteTarget{
					Package:      "example.com/pkg",
					BinaryPath:   "/tmp/pkg.test",
					SuiteName:    "TestFooSuite",
					RunFilter:    "^TestFooSuite$/^TestBar$",
					RunFlags:     []string{"-test.timeout=30s", "-test.v"},
					CoverProfile: "/tmp/cover.out",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$/^TestBar$", "-test.timeout=30s", "-test.v", "-test.coverprofile=/tmp/cover.out"},
			},
		} {
			w.It(tc.name, func(it *gotest.T) {
				cmd := gotestrunner.ExportBuildSuiteCmd(ctx, tc.target, env, false)

				gotest.Equal(it, tc.wantBinary, cmd.Path)

				// Compare full args list.
				gotest.Equal(it, len(tc.wantArgs), len(cmd.Args))
				for i := range cmd.Args {
					gotest.Equal(it, tc.wantArgs[i], cmd.Args[i])
				}

				gotest.Equal(it, len(env), len(cmd.Env))
			})
		}

		w.It("matches original buildPlainArgs", func(it *gotest.T) {
			targets := []gotestrunner.SuiteTarget{
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX"},
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$"},
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFlags: []string{"-test.v", "-test.timeout=5s"}},
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", CoverProfile: "/c.out"},
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$", RunFlags: []string{"-test.count=2"}, CoverProfile: "/c.out"},
			}
			refCtx := context.Background()
			refEnv := []string{"A=1"}

			for _, target := range targets {
				refPath, refArgs := buildPlainArgs(target)
				cmd := gotestrunner.ExportBuildSuiteCmd(refCtx, target, refEnv, false)

				gotest.Equal(it, refPath, cmd.Args[0])
				gotArgs := cmd.Args[1:]
				gotest.Equal(it, len(refArgs), len(gotArgs))
				for i := range gotArgs {
					gotest.Equal(it, refArgs[i], gotArgs[i])
				}
			}
		})
	})

	t.When("test2json mode", func(w *gotest.T) {
		ctx := context.Background()
		env := []string{"PATH=/usr/bin", "HOME=/home/test"}

		for _, tc := range []struct {
			name       string
			target     gotestrunner.SuiteTarget
			wantBinary string
			wantArgs   []string
		}{
			{
				name: "basic suite",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFooSuite",
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$", "-test.v=test2json"},
			},
			{
				name: "run filter overrides suite name",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFooSuite",
					RunFilter:  "^TestFooSuite$/^TestBar$",
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$/^TestBar$", "-test.v=test2json"},
			},
			{
				name: "strips -test.v from run flags",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFooSuite",
					RunFlags:   []string{"-test.v", "-test.timeout=30s"},
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$", "-test.v=test2json", "-test.timeout=30s"},
			},
			{
				name: "strips -test.v=true from run flags",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFooSuite",
					RunFlags:   []string{"-test.v=true"},
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$", "-test.v=test2json"},
			},
			{
				name: "with cover profile",
				target: gotestrunner.SuiteTarget{
					Package:      "example.com/pkg",
					BinaryPath:   "/tmp/pkg.test",
					SuiteName:    "TestFooSuite",
					CoverProfile: "/tmp/cover.out",
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$", "-test.v=test2json", "-test.coverprofile=/tmp/cover.out"},
			},
			{
				name: "all fields, -test.v stripped",
				target: gotestrunner.SuiteTarget{
					Package:      "example.com/pkg",
					BinaryPath:   "/tmp/pkg.test",
					SuiteName:    "TestFooSuite",
					RunFilter:    "^TestFooSuite$/^TestBar$",
					RunFlags:     []string{"-test.v", "-test.timeout=30s", "-test.count=1"},
					CoverProfile: "/tmp/cover.out",
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$/^TestBar$", "-test.v=test2json",
					"-test.timeout=30s", "-test.count=1",
					"-test.coverprofile=/tmp/cover.out"},
			},
			{
				name: "suite name with regex-special chars",
				target: gotestrunner.SuiteTarget{
					Package:    "example.com/pkg",
					BinaryPath: "/tmp/pkg.test",
					SuiteName:  "TestFoo.Bar+Baz",
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFoo\\.Bar\\+Baz$", "-test.v=test2json"},
			},
		} {
			w.It(tc.name, func(it *gotest.T) {
				cmd := gotestrunner.ExportBuildSuiteCmd(ctx, tc.target, env, true)

				// For "go", cmd.Path is resolved to the absolute path; compare Args[0] loosely.
				gotest.True(it, strings.HasSuffix(cmd.Path, "/go") || cmd.Path == "go",
					"binary: got %q, want suffix /go", cmd.Path)

				// Compare full args list.
				gotest.Equal(it, len(tc.wantArgs), len(cmd.Args))
				for i := range cmd.Args {
					if i == 0 {
						gotest.True(it, strings.HasSuffix(cmd.Args[0], "/go") || cmd.Args[0] == "go",
							"args[0]: got %q, want suffix /go", cmd.Args[0])
						continue
					}
					gotest.Equal(it, tc.wantArgs[i], cmd.Args[i])
				}

				gotest.Equal(it, len(env), len(cmd.Env))
			})
		}

		w.It("matches original buildTest2JSONArgs", func(it *gotest.T) {
			targets := []gotestrunner.SuiteTarget{
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX"},
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$"},
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFlags: []string{"-test.v", "-test.timeout=5s"}},
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFlags: []string{"-test.v=true"}},
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", CoverProfile: "/c.out"},
				{Package: "a/b", BinaryPath: "/bin/t", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$", RunFlags: []string{"-test.v", "-test.count=2"}, CoverProfile: "/c.out"},
			}
			refCtx := context.Background()
			refEnv := []string{"A=1"}

			for _, target := range targets {
				_, refArgs := buildTest2JSONArgs(target)
				cmd := gotestrunner.ExportBuildSuiteCmd(refCtx, target, refEnv, true)

				// cmd.Args[1:] against refArgs (which doesn't include "go").
				gotArgs := cmd.Args[1:]
				gotest.Equal(it, len(refArgs), len(gotArgs))
				for i := range gotArgs {
					gotest.Equal(it, refArgs[i], gotArgs[i])
				}
			}
		})
	})

	t.When("resolving go binary", func(w *gotest.T) {
		w.It("resolves go to full path in test2json mode", func(it *gotest.T) {
			ctx := context.Background()
			target := gotestrunner.SuiteTarget{
				Package:    "example.com/pkg",
				BinaryPath: "/tmp/pkg.test",
				SuiteName:  "TestFoo",
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, true)

			goPath, err := exec.LookPath("go")
			if err != nil {
				it.T().Skip("go not in PATH")
			}
			gotest.Equal(it, goPath, cmd.Path)
		})
	})
}

// --- PackageBatcher tests ---

func (s *GotestrunnerTestSuite) TestPackageBatcher(t *gotest.T) {
	t.When("recording results", func(w *gotest.T) {
		w.It("returns true only when all suites are recorded", func(it *gotest.T) {
			b := gotestrunner.NewPackageBatcher()
			b.Register("pkg/a", 3)
			b.Register("pkg/b", 1)

			r := gotestrunner.SuiteResult{ExitCode: 0}

			gotest.False(it, b.Record("pkg/a", 0, r))
			gotest.False(it, b.Record("pkg/a", 2, r))
			gotest.True(it, b.Record("pkg/a", 1, r))
			gotest.True(it, b.Record("pkg/b", 0, r))
		})
	})

	t.When("flushing mixed pass/fail results", func(w *gotest.T) {
		w.It("writes combined stdout and stderr", func(it *gotest.T) {
			b := gotestrunner.NewPackageBatcher()
			b.Register("example.com/pkg", 2)

			b.Record("example.com/pkg", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestA\n--- PASS: TestA (0.00s)\nPASS\n"),
				ExitCode: 0,
				Duration: 100 * time.Millisecond,
			})
			b.Record("example.com/pkg", 1, gotestrunner.SuiteResult{
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
			gotest.Equal(it, wantOut, bufOut.String())
			gotest.Equal(it, "some error\n", bufErr.String())
		})
	})

	t.When("flushing all-passing results", func(w *gotest.T) {
		w.It("writes PASS summary", func(it *gotest.T) {
			b := gotestrunner.NewPackageBatcher()
			b.Register("example.com/ok", 1)
			b.Record("example.com/ok", 0, gotestrunner.SuiteResult{
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
			gotest.Equal(it, wantOut, buf.String())
		})
	})
}

// --- output formatting tests ---

func (s *GotestrunnerTestSuite) TestOutputFormatting(t *gotest.T) {
	t.When("stripping trailing status", func(w *gotest.T) {
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
			w.It(tc.name, func(it *gotest.T) {
				got := gotestrunner.StripTrailingStatus([]byte(tc.input))
				if tc.expect == "" {
					gotest.True(it, got == nil, "expected nil, got %q", got)
				} else {
					gotest.Equal(it, tc.expect, string(got))
				}
			})
		}
	})

	t.When("writing package summary", func(w *gotest.T) {
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
			w.It(tc.name, func(it *gotest.T) {
				r, wr, _ := os.Pipe()
				old := os.Stdout
				os.Stdout = wr
				gotestrunner.WritePackageSummary(tc.pkg, tc.failed, tc.duration)
				wr.Close()
				os.Stdout = old
				var buf bytes.Buffer
				buf.ReadFrom(r)
				r.Close()
				gotest.Equal(it, tc.expect, buf.String())
			})
		}
	})
}

// --- splitTopLevelOr tests ---

func (s *GotestrunnerTestSuite) TestSplitTopLevelOr(t *gotest.T) {
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
		t.When(tc.name, func(w *gotest.T) {
			w.It("splits correctly", func(it *gotest.T) {
				got := gotestrunner.ExportSplitTopLevelOr(tc.input)
				gotest.Equal(it, tc.expect, got)
			})
		})
	}
}

// --- suiteRunFilter tests ---

func (s *GotestrunnerTestSuite) TestSuiteRunFilter(t *gotest.T) {
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
		t.When(tc.name, func(w *gotest.T) {
			w.It("returns the expected filter", func(it *gotest.T) {
				got := gotestrunner.ExportSuiteRunFilter(tc.userFilter, tc.testFuncName)
				gotest.Equal(it, tc.expect, got)
			})
		})
	}
}
