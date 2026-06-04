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
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
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
	for sub, tc := range gotest.Each(t, []struct {
		Name    string
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
	}) {
		isValue, known := gotestrunner.IsGoTestFlag(tc.flag)
		gotest.Equal(sub, tc.isValue, isValue)
		gotest.Equal(sub, tc.known, known)
	}
}

func (s *GotestrunnerTestSuite) TestCoverProfile(t *gotest.T) {
	t.When("extracting from flags", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Name   string
			flags  []string
			expect string
		}{
			{"empty", nil, ""},
			{"equals form", []string{"-v", "-coverprofile=cover.out"}, "cover.out"},
			{"space form", []string{"-coverprofile", "cover.out", "-v"}, "cover.out"},
			{"stops at -args", []string{"-args", "-coverprofile=cover.out"}, ""},
			{"no coverprofile", []string{"-v", "-count=1"}, ""},
		}) {
			got := gotestrunner.ExtractCoverProfile(tc.flags)
			gotest.Equal(sub, tc.expect, got)
		}
	})

	t.When("stripping from flags", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Name   string
			flags  []string
			expect []string
		}{
			{"empty", nil, nil},
			{"equals form", []string{"-v", "-coverprofile=cover.out", "-count=1"}, []string{"-v", "-count=1"}},
			{"space form", []string{"-coverprofile", "cover.out", "-v"}, []string{"-v"}},
			{"preserves -args passthrough", []string{"-v", "-args", "-coverprofile=x"}, []string{"-v", "-args", "-coverprofile=x"}},
			{"no coverprofile unchanged", []string{"-v", "-count=1"}, []string{"-v", "-count=1"}},
		}) {
			got := gotestrunner.StripCoverProfile(tc.flags)
			gotest.Equal(sub, tc.expect, got)
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
				gotest.Contains(it, lines, "foo/bar.go:10.1,12.5 1 0")
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

				data := gotest.Must(os.ReadFile(out))
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

// --- overlay cache tests ---

func (s *GotestrunnerTestSuite) TestOverlayCache(t *gotest.T) {
	t.When("computing content hash", func(w *gotest.T) {
		w.It("produces deterministic hash for same content", func(it *gotest.T) {
			results := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("package a\n")},
				{AbsPath: "/pkg/b", PTest: []byte("package b\n"), PXTest: []byte("package b_test\n")},
			}
			h1 := gotestrunner.ExportOverlayContentHash(results)
			h2 := gotestrunner.ExportOverlayContentHash(results)
			gotest.Equal(it, h1, h2)
			gotest.Equal(it, 64, len(h1))
		})

		w.It("is order-independent (sorted by AbsPath)", func(it *gotest.T) {
			r1 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("aaa")},
				{AbsPath: "/pkg/b", PTest: []byte("bbb")},
			}
			r2 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/b", PTest: []byte("bbb")},
				{AbsPath: "/pkg/a", PTest: []byte("aaa")},
			}
			gotest.Equal(it, gotestrunner.ExportOverlayContentHash(r1), gotestrunner.ExportOverlayContentHash(r2))
		})

		w.It("changes when content changes", func(it *gotest.T) {
			r1 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("version1")},
			}
			r2 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("version2")},
			}
			gotest.NotEqual(it, gotestrunner.ExportOverlayContentHash(r1), gotestrunner.ExportOverlayContentHash(r2))
		})

		w.It("changes when AbsPath changes", func(it *gotest.T) {
			r1 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/a", PTest: []byte("same")},
			}
			r2 := gotestgen.GenerateResults{
				{AbsPath: "/pkg/b", PTest: []byte("same")},
			}
			gotest.NotEqual(it, gotestrunner.ExportOverlayContentHash(r1), gotestrunner.ExportOverlayContentHash(r2))
		})

		w.It("returns empty-stable hash for nil results", func(it *gotest.T) {
			h1 := gotestrunner.ExportOverlayContentHash(nil)
			h2 := gotestrunner.ExportOverlayContentHash(gotestgen.GenerateResults{})
			gotest.Equal(it, h1, h2)
			gotest.Equal(it, 64, len(h1))
		})
	})

	t.When("cache root resolution", func(w *gotest.T) {
		w.It("respects GOTEST_CACHE_DIR env var", func(it *gotest.T) {
			dir := it.T().TempDir()
			it.T().Setenv(protocol.EnvCacheDir, dir)

			root, err := gotestrunner.ExportCacheRoot()
			gotest.NoError(it, err)
			gotest.Equal(it, dir, root)
		})

		w.It("falls back to UserCacheDir/gotest when env is unset", func(it *gotest.T) {
			it.T().Setenv(protocol.EnvCacheDir, "")

			root, err := gotestrunner.ExportCacheRoot()
			gotest.NoError(it, err)
			gotest.True(it, strings.HasSuffix(root, filepath.Join("gotest")))
			gotest.NotEmpty(it, root)
		})
	})

	t.When("writing to cache", func(w *gotest.T) {
		w.It("creates valid overlay in cache directory", func(it *gotest.T) {
			cacheDir := it.T().TempDir()
			it.T().Setenv(protocol.EnvCacheDir, cacheDir)

			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/m", PTest: []byte("package m\n"), PXTest: []byte("package m_test\n")},
			}

			hash := gotestrunner.ExportOverlayContentHash(results)
			expectedDir := filepath.Join(cacheDir, "overlays", hash)

			// Ensure it doesn't exist yet.
			_, err := os.Stat(expectedDir)
			gotest.True(it, os.IsNotExist(err))

			// Write via the exported WriteOverlay — this uses tmpdir path.
			// For cache path, we test the internal via GenerateOverlay with noCache=false.
			// Since GenerateOverlay needs loaded packages, test the cache write separately.
			// We'll use writeOverlayCached indirectly by calling the exported function.
			dir, err := gotestrunner.ExportWriteOverlayCached(results, false)
			gotest.NoError(it, err)

			gotest.Equal(it, expectedDir, dir)

			// Verify overlay.json exists and is valid.
			data, err := os.ReadFile(filepath.Join(dir, "overlay.json"))
			gotest.NoError(it, err)
			var ov gotestrunner.ExportOverlayJSON
			gotest.NoError(it, json.Unmarshal(data, &ov))
			gotest.Equal(it, 2, len(ov.Replace))
		})

		w.It("returns same directory on repeated calls (cache hit)", func(it *gotest.T) {
			cacheDir := it.T().TempDir()
			it.T().Setenv(protocol.EnvCacheDir, cacheDir)

			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/r", PTest: []byte("package r\n")},
			}

			dir1, err := gotestrunner.ExportWriteOverlayCached(results, false)
			gotest.NoError(it, err)

			dir2, err := gotestrunner.ExportWriteOverlayCached(results, false)
			gotest.NoError(it, err)

			gotest.Equal(it, dir1, dir2)
		})

		w.It("falls back to tmpdir when noCache is true", func(it *gotest.T) {
			cacheDir := it.T().TempDir()
			it.T().Setenv(protocol.EnvCacheDir, cacheDir)

			results := gotestgen.GenerateResults{
				{AbsPath: "/fake/pkg/nc", PTest: []byte("package nc\n")},
			}

			dir, err := gotestrunner.ExportWriteOverlayCached(results, true)
			gotest.NoError(it, err)
			defer os.RemoveAll(dir)

			// Should be in tmpdir, not in cache.
			gotest.False(it, strings.HasPrefix(dir, cacheDir))
		})
	})

	t.When("cleaning old cache entries", func(w *gotest.T) {
		w.It("removes entries older than 7 days", func(it *gotest.T) {
			cacheDir := it.T().TempDir()
			it.T().Setenv(protocol.EnvCacheDir, cacheDir)

			overlaysDir := filepath.Join(cacheDir, "overlays")
			os.MkdirAll(overlaysDir, 0755)

			// Create an old entry.
			oldDir := filepath.Join(overlaysDir, "old-hash-entry")
			os.MkdirAll(oldDir, 0755)
			os.WriteFile(filepath.Join(oldDir, "overlay.json"), []byte("{}"), 0644)
			oldTime := time.Now().Add(-8 * 24 * time.Hour)
			os.Chtimes(oldDir, oldTime, oldTime)

			// Create a fresh entry.
			freshDir := filepath.Join(overlaysDir, "fresh-hash-entry")
			os.MkdirAll(freshDir, 0755)
			os.WriteFile(filepath.Join(freshDir, "overlay.json"), []byte("{}"), 0644)

			gotestrunner.CleanStaleOverlays()

			_, err := os.Stat(oldDir)
			gotest.True(it, os.IsNotExist(err), "expected old cache entry to be removed")

			_, err = os.Stat(freshDir)
			gotest.False(it, os.IsNotExist(err), "expected fresh cache entry to be kept")
		})
	})
}

// --- buildSuiteCmd tests ---

func (s *GotestrunnerTestSuite) TestBuildSuiteCmd(t *gotest.T) {
	t.When("plain mode", func(w *gotest.T) {
		ctx := context.Background()
		env := []string{"PATH=/usr/bin", "HOME=/home/test"}

		for sub, tc := range gotest.Each(w, []struct {
			Name       string
			target     gotestrunner.SuiteTarget
			wantBinary string
			wantArgs   []string
		}{
			{
				Name: "basic suite",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
					BinaryPath: "/tmp/pkg.test",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$"},
			},
			{
				Name: "run filter overrides suite name",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite", RunFilter: "^TestFooSuite$/^TestBar$"},
					BinaryPath: "/tmp/pkg.test",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$/^TestBar$"},
			},
			{
				Name: "with run flags",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
					BinaryPath: "/tmp/pkg.test",
					RunFlags:   []string{"-test.timeout=30s", "-test.count=1"},
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$", "-test.timeout=30s", "-test.count=1"},
			},
			{
				Name: "keeps -test.v in run flags",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
					BinaryPath: "/tmp/pkg.test",
					RunFlags:   []string{"-test.v", "-test.timeout=10s"},
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$", "-test.v", "-test.timeout=10s"},
			},
			{
				Name: "with cover profile",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:    gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
					BinaryPath:   "/tmp/pkg.test",
					CoverProfile: "/tmp/cover.out",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$", "-test.coverprofile=/tmp/cover.out"},
			},
			{
				Name: "suite name with regex-special chars",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo.Bar+Baz"},
					BinaryPath: "/tmp/pkg.test",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFoo\\.Bar\\+Baz$"},
			},
			{
				Name: "all fields populated",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:    gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite", RunFilter: "^TestFooSuite$/^TestBar$"},
					BinaryPath:   "/tmp/pkg.test",
					RunFlags:     []string{"-test.timeout=30s", "-test.v"},
					CoverProfile: "/tmp/cover.out",
				},
				wantBinary: "/tmp/pkg.test",
				wantArgs:   []string{"/tmp/pkg.test", "-test.run=^TestFooSuite$/^TestBar$", "-test.timeout=30s", "-test.v", "-test.coverprofile=/tmp/cover.out"},
			},
		}) {
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, tc.target, env, false)

			gotest.Equal(sub, tc.wantBinary, cmd.Path)

			// Compare full args list.
			gotest.Equal(sub, len(tc.wantArgs), len(cmd.Args))
			for i := range cmd.Args {
				gotest.Equal(sub, tc.wantArgs[i], cmd.Args[i])
			}

			gotest.Equal(sub, len(env), len(cmd.Env))
		}

		w.It("matches original buildPlainArgs", func(it *gotest.T) {
			targets := []gotestrunner.SuiteTarget{
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX"}, BinaryPath: "/bin/t"},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$"}, BinaryPath: "/bin/t"},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX"}, BinaryPath: "/bin/t", RunFlags: []string{"-test.v", "-test.timeout=5s"}},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX"}, BinaryPath: "/bin/t", CoverProfile: "/c.out"},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$"}, BinaryPath: "/bin/t", RunFlags: []string{"-test.count=2"}, CoverProfile: "/c.out"},
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

		for sub, tc := range gotest.Each(w, []struct {
			Name       string
			target     gotestrunner.SuiteTarget
			wantBinary string
			wantArgs   []string
		}{
			{
				Name: "basic suite",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
					BinaryPath: "/tmp/pkg.test",
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$", "-test.v=test2json"},
			},
			{
				Name: "run filter overrides suite name",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite", RunFilter: "^TestFooSuite$/^TestBar$"},
					BinaryPath: "/tmp/pkg.test",
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$/^TestBar$", "-test.v=test2json"},
			},
			{
				Name: "strips -test.v from run flags",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
					BinaryPath: "/tmp/pkg.test",
					RunFlags:   []string{"-test.v", "-test.timeout=30s"},
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$", "-test.v=test2json", "-test.timeout=30s"},
			},
			{
				Name: "strips -test.v=true from run flags",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
					BinaryPath: "/tmp/pkg.test",
					RunFlags:   []string{"-test.v=true"},
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$", "-test.v=test2json"},
			},
			{
				Name: "with cover profile",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:    gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
					BinaryPath:   "/tmp/pkg.test",
					CoverProfile: "/tmp/cover.out",
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFooSuite$", "-test.v=test2json", "-test.coverprofile=/tmp/cover.out"},
			},
			{
				Name: "all fields, -test.v stripped",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:    gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite", RunFilter: "^TestFooSuite$/^TestBar$"},
					BinaryPath:   "/tmp/pkg.test",
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
				Name: "suite name with regex-special chars",
				target: gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo.Bar+Baz"},
					BinaryPath: "/tmp/pkg.test",
				},
				wantBinary: "go",
				wantArgs: []string{"go", "tool", "test2json", "-p", "example.com/pkg", "-t", "/tmp/pkg.test",
					"-test.run=^TestFoo\\.Bar\\+Baz$", "-test.v=test2json"},
			},
		}) {
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, tc.target, env, true)

			// For "go", cmd.Path is resolved to the absolute path; compare base name loosely.
			base := filepath.Base(cmd.Path)
			gotest.True(sub, base == "go" || base == "go.exe",
				"binary: got %q, want go or go.exe", cmd.Path)

			// Compare full args list.
			gotest.Equal(sub, len(tc.wantArgs), len(cmd.Args))
			for i := range cmd.Args {
				if i == 0 {
					a0 := filepath.Base(cmd.Args[0])
					gotest.True(sub, a0 == "go" || a0 == "go.exe",
						"args[0]: got %q, want go or go.exe", cmd.Args[0])
					continue
				}
				gotest.Equal(sub, tc.wantArgs[i], cmd.Args[i])
			}

			gotest.Equal(sub, len(env), len(cmd.Env))
		}

		w.It("matches original buildTest2JSONArgs", func(it *gotest.T) {
			targets := []gotestrunner.SuiteTarget{
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX"}, BinaryPath: "/bin/t"},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$"}, BinaryPath: "/bin/t"},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX"}, BinaryPath: "/bin/t", RunFlags: []string{"-test.v", "-test.timeout=5s"}},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX"}, BinaryPath: "/bin/t", RunFlags: []string{"-test.v=true"}},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX"}, BinaryPath: "/bin/t", CoverProfile: "/c.out"},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "a/b", SuiteName: "TestX", RunFilter: "^TestX$/^Sub$"}, BinaryPath: "/bin/t", RunFlags: []string{"-test.v", "-test.count=2"}, CoverProfile: "/c.out"},
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
				SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo"},
				BinaryPath: "/tmp/pkg.test",
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

// --- OutputCollector tests ---

func (s *GotestrunnerTestSuite) TestOutputCollector(t *gotest.T) {
	pass := func(d time.Duration) gotestrunner.SuiteResult {
		return gotestrunner.SuiteResult{Stdout: []byte("PASS\n"), ExitCode: 0, Duration: d}
	}

	t.When("flushing in registration order", func(w *gotest.T) {
		w.It("buffers later packages until earlier ones complete", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.Register("example.com/b", 1)
			c.Register("example.com/c", 1)

			c.RecordResult("example.com/c", 0, pass(30*time.Millisecond))
			gotest.Equal(it, "", stdout.String(), "c should be buffered because a and b are not done")

			c.RecordResult("example.com/a", 0, pass(10*time.Millisecond))
			gotest.Equal(it, "ok  \texample.com/a\t0.010s\n", stdout.String(), "a should flush as the head")

			stdout.Reset()
			c.RecordResult("example.com/b", 0, pass(20*time.Millisecond))
			want := "ok  \texample.com/b\t0.020s\n" +
				"ok  \texample.com/c\t0.030s\n"
			gotest.Equal(it, want, stdout.String(), "b and c should flush together")
		})

		w.It("flushes immediately when packages complete in order", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/x", 1)
			c.Register("example.com/y", 1)

			c.RecordResult("example.com/x", 0, pass(10*time.Millisecond))
			gotest.Equal(it, "ok  \texample.com/x\t0.010s\n", stdout.String())

			stdout.Reset()
			c.RecordResult("example.com/y", 0, pass(20*time.Millisecond))
			gotest.Equal(it, "ok  \texample.com/y\t0.020s\n", stdout.String())
		})
	})

	t.When("tracking failures", func(w *gotest.T) {
		w.It("reports no failure when all pass", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			c.Register("example.com/ok", 1)
			c.RecordResult("example.com/ok", 0, gotestrunner.SuiteResult{ExitCode: 0})
			gotest.False(it, c.AnyFailed())
		})

		w.It("reports failure when any suite fails", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.Register("example.com/b", 1)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{ExitCode: 0})
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{ExitCode: 1})
			gotest.True(it, c.AnyFailed())
		})

		w.It("tracks worst exit code", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.Register("example.com/b", 1)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{ExitCode: 1})
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{ExitCode: 2})
			gotest.Equal(it, 2, c.WorstExitCode())
		})
	})

	t.When("finalize", func(w *gotest.T) {
		w.It("is a no-op for captured JSON mode", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{ExitCode: 1})
			stdout.Reset()
			c.Finalize([]string{"example.com/empty"})
			gotest.Equal(it, "", stdout.String())
		})
	})

	t.When("JSON capture mode", func(w *gotest.T) {
		w.It("deduplicates package events across suites", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			c.Register("example.com/pkg", 2)

			suite1JSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/pkg","Test":"TestA"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/pkg","Test":"TestA","Elapsed":0.01}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/pkg","Output":"PASS\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/pkg","Elapsed":0.01}`,
			}, "\n") + "\n"
			suite2JSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/pkg","Test":"TestB"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"example.com/pkg","Test":"TestB","Elapsed":0.02}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/pkg","Output":"FAIL\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"example.com/pkg","Elapsed":0.02}`,
			}, "\n") + "\n"

			c.RecordResult("example.com/pkg", 0, gotestrunner.SuiteResult{
				Stdout: []byte(suite1JSON), ExitCode: 0, Duration: 10 * time.Millisecond,
			})
			c.RecordResult("example.com/pkg", 1, gotestrunner.SuiteResult{
				Stdout: []byte(suite2JSON), ExitCode: 1, Duration: 20 * time.Millisecond,
			})

			captured := c.CapturedJSON()
			lines := strings.Split(strings.TrimRight(string(captured), "\n"), "\n")

			// Count package-level pass/fail events (Test=="")
			var pkgVerdicts []string
			for _, line := range lines {
				var ev map[string]any
				if json.Unmarshal([]byte(line), &ev) != nil {
					continue
				}
				if ev["Test"] == nil && (ev["Action"] == "pass" || ev["Action"] == "fail") {
					pkgVerdicts = append(pkgVerdicts, ev["Action"].(string))
				}
			}

			gotest.Equal(it, 1, len(pkgVerdicts), "should have exactly one package-level verdict, got: %v", pkgVerdicts)
			gotest.Equal(it, "fail", pkgVerdicts[0], "package should be marked as fail when any suite fails")
		})
	})
}

func (s *GotestrunnerTestSuite) TestEmitSkippedSuites(t *gotest.T) {
	t.When("text mode", func(w *gotest.T) {
		w.It("produces no output", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{
				"example.com/a": {"SkippedSuite"},
			})
			gotest.Empty(it, stdout.String())
		})
	})

	t.When("JSON streaming mode", func(w *gotest.T) {
		w.It("emits run, output, output, skip events per suite", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{
				"example.com/pkg": {"FooSuite"},
			})

			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			gotest.Equal(it, 4, len(lines))

			var ev0, ev1, ev2, ev3 map[string]any
			gotest.NoError(it, json.Unmarshal([]byte(lines[0]), &ev0))
			gotest.NoError(it, json.Unmarshal([]byte(lines[1]), &ev1))
			gotest.NoError(it, json.Unmarshal([]byte(lines[2]), &ev2))
			gotest.NoError(it, json.Unmarshal([]byte(lines[3]), &ev3))

			gotest.Equal(it, "run", ev0["Action"])
			gotest.Equal(it, "TestFooSuite", ev0["Test"])
			gotest.Equal(it, "example.com/pkg", ev0["Package"])

			gotest.Equal(it, "output", ev1["Action"])
			gotest.Contains(it, ev1["Output"].(string), "SKIP")

			gotest.Equal(it, "output", ev2["Action"])
			gotest.Contains(it, ev2["Output"].(string), "excluded by user")

			gotest.Equal(it, "skip", ev3["Action"])
			gotest.Equal(it, "TestFooSuite", ev3["Test"])
		})

		w.It("sorts packages for deterministic output", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{
				"example.com/z": {"ZSuite"},
				"example.com/a": {"ASuite"},
			})

			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			gotest.Equal(it, 8, len(lines))

			var first, fifth map[string]any
			json.Unmarshal([]byte(lines[0]), &first)
			json.Unmarshal([]byte(lines[4]), &fifth)
			gotest.Equal(it, "example.com/a", first["Package"])
			gotest.Equal(it, "example.com/z", fifth["Package"])
		})
	})

	t.When("empty map", func(w *gotest.T) {
		w.It("produces no output", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{})
			gotest.Empty(it, stdout.String())
		})

		w.It("handles nil map", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(nil)
			gotest.Empty(it, stdout.String())
		})
	})
}

func capturePackageSummary(pkg string, failed bool, d time.Duration, verbose bool) string {
	r, wr, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = wr
	gotestrunner.WritePackageSummary(pkg, failed, d, verbose)
	wr.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	r.Close()
	return buf.String()
}


// --- output formatting tests ---

func (s *GotestrunnerTestSuite) TestOutputFormatting(t *gotest.T) {
	t.When("stripping trailing status", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Name   string
			input  string
			expect string
		}{
			{
				Name:   "strips trailing PASS",
				input:  "=== RUN   TestFoo\n--- PASS: TestFoo (0.00s)\nPASS\n",
				expect: "=== RUN   TestFoo\n--- PASS: TestFoo (0.00s)\n",
			},
			{
				Name:   "strips trailing FAIL",
				input:  "=== RUN   TestFoo\n--- FAIL: TestFoo (0.00s)\nFAIL\n",
				expect: "=== RUN   TestFoo\n--- FAIL: TestFoo (0.00s)\n",
			},
			{
				Name:   "strips trailing PASS with extra newlines",
				input:  "line1\nline2\nPASS\n\n\n",
				expect: "line1\nline2\n",
			},
			{
				Name:   "preserves non-status last line",
				input:  "line1\nline2\nsome output\n",
				expect: "line1\nline2\nsome output\n",
			},
			{
				Name:   "only PASS returns nil",
				input:  "PASS\n",
				expect: "",
			},
			{
				Name:   "no newlines returns nil",
				input:  "PASS",
				expect: "",
			},
		}) {
			got := gotestrunner.StripTrailingStatus([]byte(tc.input))
			if tc.expect == "" {
				gotest.True(sub, got == nil, "expected nil, got %q", got)
			} else {
				gotest.Equal(sub, tc.expect, string(got))
			}
		}
	})

	t.When("writing package summary", func(w *gotest.T) {
		w.When("verbose mode", func(w2 *gotest.T) {
			for sub, tc := range gotest.Each(w2, []struct {
				Name     string
				pkg      string
				failed   bool
				duration time.Duration
				expect   string
			}{
				{
					Name:     "passing package includes PASS prefix",
					pkg:      "example.com/pkg",
					failed:   false,
					duration: 1234 * time.Millisecond,
					expect:   "PASS\nok  \texample.com/pkg\t1.234s\n",
				},
				{
					Name:     "failing package includes FAIL prefix",
					pkg:      "example.com/pkg",
					failed:   true,
					duration: 567 * time.Millisecond,
					expect:   "FAIL\nFAIL\texample.com/pkg\t0.567s\n",
				},
			}) {
				got := capturePackageSummary(tc.pkg, tc.failed, tc.duration, true)
				gotest.Equal(sub, tc.expect, got)
			}
		})

		w.When("non-verbose mode", func(w2 *gotest.T) {
			for sub, tc := range gotest.Each(w2, []struct {
				Name     string
				pkg      string
				failed   bool
				duration time.Duration
				expect   string
			}{
				{
					Name:     "passing package omits PASS prefix",
					pkg:      "example.com/pkg",
					failed:   false,
					duration: 1234 * time.Millisecond,
					expect:   "ok  \texample.com/pkg\t1.234s\n",
				},
				{
					Name:     "failing package still includes FAIL prefix",
					pkg:      "example.com/pkg",
					failed:   true,
					duration: 567 * time.Millisecond,
					expect:   "FAIL\nFAIL\texample.com/pkg\t0.567s\n",
				},
			}) {
				got := capturePackageSummary(tc.pkg, tc.failed, tc.duration, false)
				gotest.Equal(sub, tc.expect, got)
			}
		})
	})

}

// --- splitTopLevelOr tests ---

func (s *GotestrunnerTestSuite) TestSplitTopLevelOr(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
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
	}) {
		got := gotestrunner.ExportSplitTopLevelOr(tc.input)
		gotest.Equal(sub, tc.expect, got)
	}
}

// --- HasVerboseFlag tests ---

func (s *GotestrunnerTestSuite) TestHasVerboseFlag(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		flags  []string
		expect bool
	}{
		{"empty flags", nil, false},
		{"no -v", []string{"-count=1", "-timeout=10m"}, false},
		{"-v present", []string{"-count=1", "-v"}, true},
		{"-v=true present", []string{"-v=true", "-timeout=10m"}, true},
		{"-v=false is not verbose", []string{"-v=false"}, false},
		{"-verbose is not -v", []string{"-verbose"}, false},
	}) {
		got := gotestrunner.HasVerboseFlag(tc.flags)
		gotest.Equal(sub, tc.expect, got)
	}
}

// --- suiteRunFilter tests ---

func (s *GotestrunnerTestSuite) TestSuiteRunFilter(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name         string
		userFilter   string
		testFuncName string
		expect       string
	}{
		{
			Name:         "empty filter",
			userFilter:   "",
			testFuncName: "TestFooSuite",
			expect:       "",
		},
		{
			Name:         "suite only (no subtest)",
			userFilter:   "^TestFooSuite$",
			testFuncName: "TestFooSuite",
			expect:       "",
		},
		{
			Name:         "single method filter",
			userFilter:   "^TestFooSuite$/^TestBar$",
			testFuncName: "TestFooSuite",
			expect:       "^TestFooSuite$/^TestBar$",
		},
		{
			Name:         "multi-method same suite",
			userFilter:   "^TestFooSuite$/^(TestBar|TestBaz)$",
			testFuncName: "TestFooSuite",
			expect:       "^TestFooSuite$/^(TestBar|TestBaz)$",
		},
		{
			Name:         "multi-suite picks matching",
			userFilter:   "^TestSuiteA$/^TestX$|^TestSuiteB$/^TestY$",
			testFuncName: "TestSuiteA",
			expect:       "^TestSuiteA$/^TestX$",
		},
		{
			Name:         "multi-suite picks other",
			userFilter:   "^TestSuiteA$/^TestX$|^TestSuiteB$/^TestY$",
			testFuncName: "TestSuiteB",
			expect:       "^TestSuiteB$/^TestY$",
		},
		{
			Name:         "multi-suite no match",
			userFilter:   "^TestSuiteA$/^TestX$|^TestSuiteB$/^TestY$",
			testFuncName: "TestSuiteC",
			expect:       "",
		},
	}) {
		got := gotestrunner.ExportSuiteRunFilter(tc.userFilter, tc.testFuncName)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *GotestrunnerTestSuite) TestComputeConcurrency(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name         string
		budget       int
		numSuites    int
		gomaxprocs   int
		wantInter    int
		wantIntra    int
	}{
		// Default budget (2×GOMAXPROCS), 4 cores
		{"1 suite 4 cores", 8, 1, 4, 1, 8},
		{"2 suites 4 cores", 8, 2, 4, 2, 4},
		{"4 suites 4 cores", 8, 4, 4, 4, 2},
		{"20 suites 4 cores", 8, 20, 4, 4, 2},

		// Default budget (2×GOMAXPROCS), 8 cores
		{"2 suites 8 cores", 16, 2, 8, 2, 8},
		{"8 suites 8 cores", 16, 8, 8, 8, 2},
		{"30 suites 8 cores", 16, 30, 8, 8, 2},

		// Small budget (< GOMAXPROCS)
		{"budget 4 on 8 cores", 4, 20, 8, 4, 1},
		{"budget 2 on 8 cores", 2, 20, 8, 2, 1},

		// Large budget
		{"budget 32 on 4 cores", 32, 20, 4, 4, 8},

		// Edge: fewer suites than cores
		{"budget 16 1 suite", 16, 1, 8, 1, 16},

		// Edge: zero/negative budget uses default
		{"zero budget", 0, 10, 4, 4, 2},

		// Edge: zero suites
		{"zero suites", 8, 0, 4, 1, 8},
	}) {
		inter, intra := gotestrunner.ComputeConcurrency(tc.budget, tc.numSuites, tc.gomaxprocs)
		gotest.Equal(sub, tc.wantInter, inter, "inter")
		gotest.Equal(sub, tc.wantIntra, intra, "intra")
	}
}

func (s *GotestrunnerTestSuite) TestExtractParallelValue(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		flags  []string
		expect int
	}{
		{"empty", nil, 0},
		{"not present", []string{"-v", "-timeout=10m"}, 0},
		{"equals form", []string{"-v", "-parallel=4"}, 4},
		{"space form", []string{"-parallel", "8", "-v"}, 8},
		{"stops at -args", []string{"-args", "-parallel=4"}, 0},
		{"invalid value", []string{"-parallel=abc"}, 0},
	}) {
		got := gotestrunner.ExtractParallelValue(tc.flags)
		gotest.Equal(sub, tc.expect, got)
	}
}

func (s *GotestrunnerTestSuite) TestInjectParallel(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name   string
		flags  []string
		n      int
		expect []string
	}{
		{"injects into empty", nil, 4, []string{"-parallel=4"}},
		{"injects when absent", []string{"-v", "-timeout=10m"}, 2, []string{"-v", "-timeout=10m", "-parallel=2"}},
		{"skips when equals present", []string{"-parallel=8", "-v"}, 2, []string{"-parallel=8", "-v"}},
		{"skips when space present", []string{"-parallel", "8", "-v"}, 2, []string{"-parallel", "8", "-v"}},
		{"does not inject after -args", []string{"-v", "-args", "-parallel=9"}, 2, []string{"-v", "-args", "-parallel=9", "-parallel=2"}},
	}) {
		got := gotestrunner.InjectParallel(tc.flags, tc.n)
		gotest.Equal(sub, tc.expect, got)
	}
}

var jsonTimestampRe = regexp.MustCompile(`\d+\.\d+s`)

func normalizeJSON(raw string) string {
	var lines []string
	for line := range strings.SplitSeq(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) != nil {
			lines = append(lines, line)
			continue
		}
		ev["Time"] = "«TIME»"
		if _, ok := ev["Elapsed"]; ok {
			ev["Elapsed"] = "«ELAPSED»"
		}
		if output, ok := ev["Output"].(string); ok {
			ev["Output"] = jsonTimestampRe.ReplaceAllString(output, "«TS»")
		}
		normalized := gotest.Must(json.Marshal(ev))
		lines = append(lines, string(normalized))
	}
	return strings.Join(lines, "\n") + "\n"
}

func (s *GotestrunnerTestSuite) TestOutputGolden(t *gotest.T) {
	t.When("text non-verbose", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/ok", 1)
			c.RecordResult("example.com/ok", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("PASS\n"),
				ExitCode: 0,
				Duration: 50 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, stdout.String())
		})

		w.It("single failing package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/fail", 1)
			c.RecordResult("example.com/fail", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("--- FAIL: TestBad (0.00s)\n    bad_test.go:5: assertion failed\nFAIL\n"),
				Stderr:   []byte(""),
				ExitCode: 1,
				Duration: 100 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, stdout.String())
		})

		w.It("multi-package mixed with no-test-files", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))

			// Package A: 2 suites — first passes, second fails → package fails
			c.Register("example.com/a", 2)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("PASS\n"),
				ExitCode: 0,
				Duration: 100 * time.Millisecond,
			})
			c.RecordResult("example.com/a", 1, gotestrunner.SuiteResult{
				Stdout:   []byte("--- FAIL: TestBad (0.00s)\n    bad_test.go:5: nope\nFAIL\n"),
				ExitCode: 1,
				Duration: 200 * time.Millisecond,
			})

			// Package B: 1 suite — passes
			c.Register("example.com/b", 1)
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("PASS\n"),
				ExitCode: 0,
				Duration: 20 * time.Millisecond,
			})

			c.Finalize([]string{"example.com/c"})

			gotest.MatchSnapshot(it, stdout.String())
		})
	})

	t.When("text verbose", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, true, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/ok", 1)
			c.RecordResult("example.com/ok", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestOK\n--- PASS: TestOK (0.00s)\nPASS\n"),
				ExitCode: 0,
				Duration: 50 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, stdout.String())
		})

		w.It("single failing package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, true, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/fail", 1)
			c.RecordResult("example.com/fail", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestBad\n--- FAIL: TestBad (0.00s)\n    bad_test.go:5: assertion failed\nFAIL\n"),
				ExitCode: 1,
				Duration: 100 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, stdout.String())
		})

		w.It("multi-package mixed with no-test-files", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, true, gotestrunner.WithWriters(&stdout, &stderr))

			// Package A: 2 suites — first passes, second fails
			c.Register("example.com/a", 2)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestGoodA\n--- PASS: TestGoodA (0.00s)\nPASS\n"),
				ExitCode: 0,
				Duration: 100 * time.Millisecond,
			})
			c.RecordResult("example.com/a", 1, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestBadA\n--- FAIL: TestBadA (0.00s)\n    bad_test.go:5: nope\nFAIL\n"),
				ExitCode: 1,
				Duration: 200 * time.Millisecond,
			})

			// Package B: 1 suite — passes
			c.Register("example.com/b", 1)
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestGoodB\n--- PASS: TestGoodB (0.00s)\nPASS\n"),
				ExitCode: 0,
				Duration: 20 * time.Millisecond,
			})

			c.Finalize([]string{"example.com/c"})

			gotest.MatchSnapshot(it, stdout.String())
		})
	})

	t.When("json streaming", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/ok", 1)

			suiteJSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"start","Package":"example.com/ok"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/ok","Test":"TestFoo"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/ok","Test":"TestFoo","Output":"=== RUN   TestFoo\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/ok","Test":"TestFoo","Elapsed":0.001}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/ok","Output":"PASS\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/ok","Elapsed":0.05}`,
			}, "\n") + "\n"

			c.RecordResult("example.com/ok", 0, gotestrunner.SuiteResult{
				Stdout:   []byte(suiteJSON),
				ExitCode: 0,
				Duration: 50 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, normalizeJSON(stdout.String()))
		})

		w.It("multi-package mixed", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.Register("example.com/b", 1)

			passJSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/a","Test":"TestA"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/a","Test":"TestA","Elapsed":0.001}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/a","Output":"PASS\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/a","Elapsed":0.01}`,
			}, "\n") + "\n"

			failJSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/b","Test":"TestB"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"example.com/b","Test":"TestB","Elapsed":0.002}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/b","Output":"FAIL\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"example.com/b","Elapsed":0.02}`,
			}, "\n") + "\n"

			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{
				Stdout: []byte(passJSON), ExitCode: 0, Duration: 10 * time.Millisecond,
			})
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{
				Stdout: []byte(failJSON), ExitCode: 1, Duration: 20 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, normalizeJSON(stdout.String()))
		})
	})
}

func (s *GotestrunnerTestSuite) TestParseExecFlags(t *gotest.T) {
	t.When("parsing exec flags", func(w *gotest.T) {
		w.It("extracts verbose flag", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags([]string{"-v", "./..."})
			gotest.True(it, pf.Verbose)
		})
		w.It("extracts run filter", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags([]string{"-run", "TestFoo", "-v"})
			gotest.Equal(it, "TestFoo", pf.UserRunFilter)
		})
		w.It("extracts cover profile", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags([]string{"-coverprofile=cover.out", "-v"})
			gotest.Equal(it, "cover.out", pf.UserCoverProfile)
		})
		w.It("separates build and run flags", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags([]string{"-race", "-v", "-count=1"})
			gotest.Contains(it, pf.BuildFlags, "-race")
		})
		w.It("handles empty args", func(it *gotest.T) {
			pf := gotestrunner.ParseExecFlags(nil)
			gotest.False(it, pf.Verbose)
			gotest.Empty(it, pf.UserRunFilter)
		})
	})
}

func (s *GotestrunnerTestSuite) TestAssignCoverProfiles(t *gotest.T) {
	t.When("assigning cover profiles", func(w *gotest.T) {
		w.It("assigns sequential paths", func(it *gotest.T) {
			targets := []gotestrunner.SuiteTarget{
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "pkg/a", SuiteName: "TestA"}},
				{SuiteSpec: gotestrunner.SuiteSpec{Package: "pkg/b", SuiteName: "TestB"}},
			}
			gotestrunner.ExportAssignCoverProfiles(targets, "/tmp/cover")
			gotest.Equal(it, filepath.Join("/tmp/cover", "0.out"), targets[0].CoverProfile)
			gotest.Equal(it, filepath.Join("/tmp/cover", "1.out"), targets[1].CoverProfile)
		})
	})
}

func (s *GotestrunnerTestSuite) TestBuildExtraEnv(t *gotest.T) {
	t.When("building extra env", func(w *gotest.T) {
		w.It("includes snapshot flag when set", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{UpdateSnapshots: true}, nil)
			gotest.Equal(it, "1", env[protocol.EnvUpdateSnapshots])
		})
		w.It("omits snapshot flag when not set", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{}, nil)
			_, ok := env[protocol.EnvUpdateSnapshots]
			gotest.False(it, ok)
		})
		w.It("omits state file when no process", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{}, nil)
			_, ok := env[protocol.EnvSharedStateFile]
			gotest.False(it, ok)
		})
	})
}

func (s *GotestrunnerTestSuite) TestCIAutoDetection(t *gotest.T) {
	t.When("auto-detecting CI from environment", func(w *gotest.T) {
		w.It("sets CI when CI=true and GOTEST_CI is unset", func(it *gotest.T) {
			it.T().Setenv("CI", "true")
			it.T().Setenv(protocol.EnvCI, "")

			cfg := gotestrunner.ExportAutoDetectCI(gotestrunner.PipelineConfig{})
			gotest.True(it, cfg.CI)
		})

		w.It("does not override explicit --ci flag", func(it *gotest.T) {
			it.T().Setenv("CI", "")

			cfg := gotestrunner.ExportAutoDetectCI(gotestrunner.PipelineConfig{CI: true})
			gotest.True(it, cfg.CI)
		})

		w.It("respects GOTEST_CI=0 opt-out", func(it *gotest.T) {
			it.T().Setenv("CI", "true")
			it.T().Setenv(protocol.EnvCI, "0")

			cfg := gotestrunner.ExportAutoDetectCI(gotestrunner.PipelineConfig{})
			gotest.False(it, cfg.CI)
		})

		w.It("stays off when neither CI nor GOTEST_CI is set", func(it *gotest.T) {
			it.T().Setenv("CI", "")
			it.T().Setenv(protocol.EnvCI, "")

			cfg := gotestrunner.ExportAutoDetectCI(gotestrunner.PipelineConfig{})
			gotest.False(it, cfg.CI)
		})
	})

	t.When("propagating CI to subprocess env", func(w *gotest.T) {
		w.It("sets GOTEST_CI in extra env when CI is true", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{CI: true}, nil)
			gotest.Equal(it, "1", env[protocol.EnvCI])
		})

		w.It("omits GOTEST_CI in extra env when CI is false", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{}, nil)
			_, ok := env[protocol.EnvCI]
			gotest.False(it, ok)
		})

		w.It("appends GOTEST_CI to base env when CI is true", func(it *gotest.T) {
			it.T().Setenv("CI", "")
			it.T().Setenv(protocol.EnvCI, "")

			env := gotestrunner.ExportBuildBaseEnv(gotestrunner.PipelineConfig{CI: true})
			found := false
			for _, e := range env {
				if e == protocol.EnvCI+"=1" {
					found = true
					break
				}
			}
			gotest.True(it, found, "expected GOTEST_CI=1 in base env")
		})
	})
}

func (s *GotestrunnerTestSuite) TestSharedFixtureProcess(t *gotest.T) {
	t.When("fixtureStateEntry parsing", func(w *gotest.T) {
		w.It("parses fixture state line", func(it *gotest.T) {
			line := `{"key":"pkg.Fixture","state":{"Host":"localhost"}}`
			var entry gotestrunner.ExportFixtureStateEntry
			err := json.Unmarshal([]byte(line), &entry)
			gotest.NoError(it, err)
			gotest.Equal(it, "pkg.Fixture", entry.Key)
			gotest.NotEmpty(it, entry.State)
		})
		w.It("parses done sentinel", func(it *gotest.T) {
			line := `{"key":"_done","teardownBudget":"2m30s"}`
			var entry gotestrunner.ExportFixtureStateEntry
			err := json.Unmarshal([]byte(line), &entry)
			gotest.NoError(it, err)
			gotest.Equal(it, "_done", entry.Key)
			gotest.Equal(it, "2m30s", entry.TeardownBudget)
		})
		w.It("parses done sentinel with error", func(it *gotest.T) {
			line := `{"key":"_done","error":"one or more shared fixtures failed"}`
			var entry gotestrunner.ExportFixtureStateEntry
			err := json.Unmarshal([]byte(line), &entry)
			gotest.NoError(it, err)
			gotest.Equal(it, "_done", entry.Key)
			gotest.Equal(it, "one or more shared fixtures failed", entry.Error)
		})
	})

	t.When("WriteStateFileForKeys", func(w *gotest.T) {
		w.It("writes subset of state to file", func(it *gotest.T) {
			tmpDir := it.T().TempDir()
			proc := gotestrunner.ExportNewSharedFixtureProcess(tmpDir, map[string]json.RawMessage{
				"pkg.Alpha": json.RawMessage(`{"Value":"a"}`),
				"pkg.Beta":  json.RawMessage(`{"Value":"b"}`),
				"pkg.Gamma": json.RawMessage(`{"Value":"c"}`),
			})
			path, err := proc.WriteStateFileForKeys("TestSuite", []string{"pkg.Alpha", "pkg.Gamma"})
			gotest.NoError(it, err)
			gotest.Contains(it, path, "TestSuite.json")

			data, err := os.ReadFile(path)
			gotest.NoError(it, err)
			var state map[string]json.RawMessage
			gotest.NoError(it, json.Unmarshal(data, &state))
			gotest.Equal(it, 2, len(state))
			_, hasAlpha := state["pkg.Alpha"]
			gotest.True(it, hasAlpha)
			_, hasBeta := state["pkg.Beta"]
			gotest.False(it, hasBeta)
			_, hasGamma := state["pkg.Gamma"]
			gotest.True(it, hasGamma)
		})
	})

	t.When("State", func(w *gotest.T) {
		w.It("returns only requested keys", func(it *gotest.T) {
			proc := gotestrunner.ExportNewSharedFixtureProcess("", map[string]json.RawMessage{
				"pkg.Alpha": json.RawMessage(`{"a":1}`),
				"pkg.Beta":  json.RawMessage(`{"b":2}`),
			})
			result := proc.State([]string{"pkg.Alpha"})
			gotest.Equal(it, 1, len(result))
			_, hasAlpha := result["pkg.Alpha"]
			gotest.True(it, hasAlpha)
		})
	})
}
