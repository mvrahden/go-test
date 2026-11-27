package gotestrunner_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CompileGroupTestSuite: a run builds its packages with one go test -c per
// group of distinct binary names, and reports each package once.
// Sequential: it replaces the go command the compile stage runs.
type CompileGroupTestSuite struct{}

// fakeGo acts as one go test -c call: stage is its -o dir, pkgs its
// packages.
type fakeGo func(stage string, pkgs []string, stdout io.Writer) error

type groupCtx struct {
	out, cache string
	fake       fakeGo
	mu         sync.Mutex
	calls      [][]string
	envs       [][]string
	restore    []func()
}

func (s *CompileGroupTestSuite) BeforeEach(t *gotest.T) *groupCtx {
	ctx := &groupCtx{out: t.TempDir(), cache: t.TempDir()}
	ctx.restore = append(ctx.restore,
		gotestrunner.ExportSetGoTestC(func(_ context.Context, args, env []string, stdout, _ io.Writer) error {
			ctx.mu.Lock()
			ctx.calls = append(ctx.calls, args)
			ctx.envs = append(ctx.envs, env)
			ctx.mu.Unlock()
			stage, pkgs := fakeTarget(args)
			return ctx.fake(stage, pkgs, stdout)
		}),
		// No GOCACHE: go copies, so a slot is judged once the call returned.
		gotestrunner.ExportSetGoCacheDir(""),
	)
	return ctx
}

func (s *CompileGroupTestSuite) AfterEach(t *gotest.T, ctx *groupCtx) {
	for _, restore := range slices.Backward(ctx.restore) {
		restore()
	}
}

// fakeTarget reads the -o dir and the packages from a go test -c call.
func fakeTarget(args []string) (stage string, pkgs []string) {
	i := slices.Index(args, "-o")
	stage = strings.TrimSuffix(args[i+1], string(os.PathSeparator))
	for _, a := range args[i+2:] {
		if !strings.HasPrefix(a, "-") {
			pkgs = append(pkgs, a)
		}
	}
	return stage, pkgs
}

// callWith is the call that built exactly pkgs.
func (c *groupCtx) callWith(pkgs ...string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, call := range c.calls {
		if _, got := fakeTarget(call); slices.Equal(got, pkgs) {
			return call
		}
	}
	return nil
}

func binaryAt(stage, pkg string) string {
	return filepath.Join(stage, gotestrunner.ExportTestBinaryName(pkg, runtime.GOOS))
}

// writeBinary writes pkg's binary into an empty slot.
func writeBinary(stage, pkg, content string) {
	_ = os.WriteFile(binaryAt(stage, pkg), []byte(content), 0o600)
}

// replaceBinary removes pkg's slot, then writes it: what go does to a
// stale output.
func replaceBinary(stage, pkg, content string) {
	_ = os.Remove(binaryAt(stage, pkg))
	writeBinary(stage, pkg, content)
}

func cachedAt(cache, pkg string) string {
	return filepath.Join(cache, gotestrunner.ExportSanitizePkgName(pkg)+".test")
}

// writeCached leaves pkg's binary in the cache, recorded as an earlier run
// linked it.
func writeCached(t *gotest.T, cache, pkg, content string) {
	entry := cachedAt(cache, pkg)
	gotest.NoError(t, os.WriteFile(entry, []byte(content), 0o600))
	gotest.NoError(t, gotestrunner.ExportWriteSum(entry, gotest.Must(gotestrunner.ExportSumFile(entry))))
}

func readCached(t *gotest.T, cache, pkg string) string {
	return string(gotest.Must(os.ReadFile(cachedAt(cache, pkg))))
}

func (s *CompileGroupTestSuite) TestOneCallPerGroup(t *gotest.T, ctx *groupCtx) {
	ctx.fake = func(stage string, pkgs []string, stdout io.Writer) error {
		for _, p := range pkgs {
			writeBinary(stage, p, "built")
		}
		return nil
	}
	results, failures := gotestrunner.CompilePackages(t.Context(), []string{"m/a", "m/b", "m/x/a"}, "-overlay=o.json", []string{"-race"}, ctx.out, "", 0)
	t.It("starts one go command per group", func(it *gotest.T) { gotest.Len(it, ctx.calls, 2) })
	t.It("passes -c, -json, the overlay, the stage dir, the build flags, then the packages", func(it *gotest.T) {
		first := ctx.callWith("m/a", "m/b")
		gotest.Len(it, first, 9)
		gotest.Equal(it, []string{"test", "-c", "-json", "-overlay=o.json", "-o"}, first[:5])
		gotest.Regexp(it, `[/\\]$`, first[5])
		gotest.Equal(it, []string{"-race", "m/a", "m/b"}, first[6:])
	})
	t.It("reports every package built", func(it *gotest.T) {
		gotest.Empty(it, failures)
		gotest.Len(it, results, 3)
	})
	t.It("sets GOTMPDIR inside the run's own dir", func(it *gotest.T) {
		gotest.Contains(it, ctx.envs[0], "GOTMPDIR="+filepath.Join(ctx.out, "gotmp"))
	})
}

func (s *CompileGroupTestSuite) TestAFailedDependency(t *gotest.T, ctx *groupCtx) {
	// m/e builds; m/f and m/h import m/g, which does not compile: go says so once, under m/g.
	ctx.fake = func(stage string, pkgs []string, stdout io.Writer) error {
		if len(pkgs) == 3 {
			writeBinary(stage, "m/e", "built")
		}
		fmt.Fprint(stdout, `{"ImportPath":"m/g","Action":"build-output","Output":"# m/g\ng/g.go:3:23: undefined: undefinedG\n"}`+"\n"+`{"ImportPath":"m/g","Action":"build-fail"}`+"\n")
		return errors.New("exit status 1")
	}
	// m/f has a cached binary from an earlier run.
	writeCached(t, ctx.cache, "m/f", "stale")
	results, failures := gotestrunner.CompilePackages(t.Context(), []string{"m/e", "m/f", "m/h"}, "-overlay=o.json", nil, ctx.out, ctx.cache, 0)
	t.It("runs the package that built", func(it *gotest.T) {
		gotest.Len(it, results, 1)
		gotest.Equal(it, "m/e", results[0].Package)
	})
	t.It("builds each package left without a verdict alone", func(it *gotest.T) {
		gotest.Len(it, ctx.calls, 3) // the group, then m/f, then m/h
	})
	t.It("reports the dependency's diagnostics under each of them", func(it *gotest.T) {
		gotest.Len(it, failures, 2)
		gotest.Equal(it, "compile m/f: exit status 1\n# m/g\ng/g.go:3:23: undefined: undefinedG", failures[0].Err.Error())
		gotest.Equal(it, "m/h", failures[1].Package)
	})
	t.It("never hands out the binary the cache placed", func(it *gotest.T) {
		for _, r := range results {
			gotest.NotEqual(it, "m/f", r.Package)
		}
	})
}

func (s *CompileGroupTestSuite) TestAttributedFailures(t *gotest.T, ctx *groupCtx) {
	ctx.fake = func(stage string, pkgs []string, stdout io.Writer) error {
		writeBinary(stage, "m/ok", "built")
		fmt.Fprint(stdout, `{"ImportPath":"m/one [m/one.test]","Action":"build-output","Output":"one finding\n"}`+"\n"+`{"ImportPath":"m/one [m/one.test]","Action":"build-fail"}`+"\n")
		return errors.New("exit status 1")
	}
	results, failures := gotestrunner.CompilePackages(t.Context(), []string{"m/ok", "m/one"}, "-overlay=o.json", nil, ctx.out, "", 0)
	t.It("names the failure and makes no second call for it", func(it *gotest.T) {
		gotest.Len(it, ctx.calls, 1)
		gotest.Len(it, results, 1)
		gotest.Len(it, failures, 1)
		gotest.Equal(it, "compile m/one: exit status 1\none finding", failures[0].Err.Error())
	})
}

func (s *CompileGroupTestSuite) TestRebuiltBinariesReachTheCache(t *gotest.T, ctx *groupCtx) {
	writeCached(t, ctx.cache, "m/a", "old")
	ctx.fake = func(stage string, pkgs []string, stdout io.Writer) error {
		replaceBinary(stage, "m/a", "new")
		return nil
	}
	_, failures := gotestrunner.CompilePackages(t.Context(), []string{"m/a"}, "-overlay=o.json", nil, ctx.out, ctx.cache, 0)
	t.It("publishes the rebuilt binary", func(it *gotest.T) {
		gotest.Empty(it, failures)
		gotest.Equal(it, "new", readCached(it, ctx.cache, "m/a"))
	})
}

func (s *CompileGroupTestSuite) TestATargetPlatform(t *gotest.T, ctx *groupCtx) {
	other, name := "windows", "a.test.exe"
	if runtime.GOOS == "windows" {
		other, name = "linux", "a.test"
	}
	ctx.restore = append(ctx.restore, gotestrunner.ExportSetGoSettings(gotestrunner.GoSettings{GOOS: other, GOARCH: runtime.GOARCH}))
	ctx.fake = func(stage string, pkgs []string, _ io.Writer) error {
		return os.WriteFile(filepath.Join(stage, name), []byte("built"), 0o600)
	}
	results, failures := gotestrunner.CompilePackages(t.Context(), []string{"m/a"}, "-overlay=o.json", nil, ctx.out, "", 0)
	t.When("go builds for another operating system than the host's", func(w *gotest.T) {
		w.It("hands out the binary under the name go gives it for that system", func(it *gotest.T) {
			gotest.Empty(it, failures)
			gotest.Len(it, results, 1)
			gotest.Equal(it, name, filepath.Base(results[0].BinaryPath))
		})
	})
}
