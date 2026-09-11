package gotestrunner_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SuiteCommandTestSuite covers how a compiled package becomes suite targets and how each
// target becomes one test-binary invocation, including bench scoping.
type SuiteCommandTestSuite struct{}

func (s *SuiteCommandTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SuiteCommandTestSuite) TestBuildSuiteCmd(t *gotest.T) {
	t.When("plain mode", func(w *gotest.T) {
		ctx := context.Background()
		env := []string{"PATH=/usr/bin", "HOME=/home/test"}

		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
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
			gotest.Len(sub, tc.wantArgs, len(cmd.Args))
			for i := range cmd.Args {
				gotest.Equal(sub, tc.wantArgs[i], cmd.Args[i])
			}

			gotest.Len(sub, env, len(cmd.Env))
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

			for i := range targets { //nolint:gocritic // hugeParam: stable API
				target := targets[i]
				refPath, refArgs := buildPlainArgs(target)
				cmd := gotestrunner.ExportBuildSuiteCmd(refCtx, target, refEnv, false)

				gotest.Equal(it, refPath, cmd.Args[0])
				gotArgs := cmd.Args[1:]
				gotest.Len(it, refArgs, len(gotArgs))
				for i := range gotArgs {
					gotest.Equal(it, refArgs[i], gotArgs[i])
				}
			}
		})
	})

	t.When("test2json mode", func(w *gotest.T) {
		ctx := context.Background()
		env := []string{"PATH=/usr/bin", "HOME=/home/test"}

		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
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
			gotest.Len(sub, tc.wantArgs, len(cmd.Args))
			for i := range cmd.Args {
				if i == 0 {
					a0 := filepath.Base(cmd.Args[0])
					gotest.True(sub, a0 == "go" || a0 == "go.exe",
						"args[0]: got %q, want go or go.exe", cmd.Args[0])
					continue
				}
				gotest.Equal(sub, tc.wantArgs[i], cmd.Args[i])
			}

			gotest.Len(sub, env, len(cmd.Env))
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

			for i := range targets { //nolint:gocritic // hugeParam: stable API
				target := targets[i]
				_, refArgs := buildTest2JSONArgs(target)
				cmd := gotestrunner.ExportBuildSuiteCmd(refCtx, target, refEnv, true)

				// cmd.Args[1:] against refArgs (which doesn't include "go").
				gotArgs := cmd.Args[1:]
				gotest.Len(it, refArgs, len(gotArgs))
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
				it.Skipf("go not in PATH")
			}
			gotest.Equal(it, goPath, cmd.Path)
		})
	})

	t.When("bench mode", func(w *gotest.T) {
		ctx := context.Background()

		w.It("targets Benchmark<Suite> and disables tests", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{SuiteName: "BenchTestSuite", Package: "example.com/p", Dir: it.TempDir()},
				BinaryPath: "/tmp/bin.test",
				Bench:      true,
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, "-test.run=^$")
			gotest.Contains(it, cmd.Args, "-test.bench=^BenchmarkBenchTestSuite$")
			gotest.Contains(it, cmd.Args, "-test.benchmem")
		})

		w.It("quotes regex-special characters in the suite name", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{SuiteName: "Foo.Bar+Baz", Package: "example.com/p"},
				BinaryPath: "/tmp/bin.test",
				Bench:      true,
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, `-test.bench=^BenchmarkFoo\.Bar\+Baz$`)
		})

		w.It("does not append -test.benchmem when the user already passed one", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{SuiteName: "BenchTestSuite", Package: "example.com/p"},
				BinaryPath: "/tmp/bin.test",
				Bench:      true,
				RunFlags:   []string{"-test.benchmem", "-test.benchtime=2x"},
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			count := 0
			for _, a := range cmd.Args {
				if a == "-test.benchmem" {
					count++
				}
			}
			gotest.Equal(it, 1, count, "expected exactly one -test.benchmem, got args: %v", cmd.Args)
			gotest.Contains(it, cmd.Args, "-test.benchtime=2x")
		})

		w.It("ignores RunFilter when Bench is set", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{SuiteName: "BenchTestSuite", Package: "example.com/p", RunFilter: "^TestFoo$/^Bar$"},
				BinaryPath: "/tmp/bin.test",
				Bench:      true,
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, "-test.run=^$")
			gotest.NotContains(it, cmd.Args, "-test.run=^TestFoo$/^Bar$")
		})
	})

	t.When("fuzz seed replay", func(w *gotest.T) {
		ctx := context.Background()

		w.It("composes an alternation of the suite name and every fuzz func when no RunFilter is set", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{SuiteName: "TestFooSuite", Package: "example.com/p"},
				BinaryPath: "/tmp/bin.test",
				FuzzFuncs:  []string{"FuzzFooSuite_A", "FuzzFooSuite_B"},
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, "-test.run=^(?:TestFooSuite|FuzzFooSuite_A|FuzzFooSuite_B)$")
		})

		w.It("leaves a user RunFilter untouched even when fuzz funcs are present", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{SuiteName: "TestFooSuite", Package: "example.com/p", RunFilter: "^TestFooSuite$/^Sub$"},
				BinaryPath: "/tmp/bin.test",
				FuzzFuncs:  []string{"FuzzFooSuite_A"},
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, "-test.run=^TestFooSuite$/^Sub$")
			for _, a := range cmd.Args {
				gotest.NotContains(it, a, "FuzzFooSuite_A", "user RunFilter must not be widened with fuzz funcs, got arg: %s", a)
			}
		})

		w.It("does not affect the run arg when FuzzFuncs is empty", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{SuiteName: "TestFooSuite", Package: "example.com/p"},
				BinaryPath: "/tmp/bin.test",
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, "-test.run=^TestFooSuite$")
		})

		w.It("ignores FuzzFuncs on bench targets", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{SuiteName: "BenchTestSuite", Package: "example.com/p"},
				BinaryPath: "/tmp/bin.test",
				Bench:      true,
				FuzzFuncs:  []string{"FuzzBenchTestSuite_A"},
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, "-test.run=^$")
			gotest.Contains(it, cmd.Args, "-test.bench=^BenchmarkBenchTestSuite$")
			for _, a := range cmd.Args {
				gotest.NotContains(it, a, "FuzzBenchTestSuite_A", "bench mode must ignore FuzzFuncs, got arg: %s", a)
			}
		})

		w.It("quotes regex-special characters in fuzz func names", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{SuiteName: "TestFooSuite", Package: "example.com/p"},
				BinaryPath: "/tmp/bin.test",
				FuzzFuncs:  []string{"FuzzFoo.Bar+Baz"},
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, `-test.run=^(?:TestFooSuite|FuzzFoo\.Bar\+Baz)$`)
		})
	})
}

func (s *SuiteCommandTestSuite) TestBuildSuiteTargets(t *gotest.T) {
	compiled := []gotestrunner.CompileResult{
		{Package: "example.com/pkg", BinaryPath: "/tmp/pkg.test"},
	}
	dirsByPkg := map[string]string{"example.com/pkg": "/src/pkg"}

	t.When("populating FuzzFuncs from fuzzFuncsByPkg", func(w *gotest.T) {
		w.It("attaches the suite's fuzz func names by pkg+suite", func(it *gotest.T) {
			suitesByPkg := map[string][]string{"example.com/pkg": {"FooSuite"}}
			fuzzFuncsByPkg := map[string]map[string][]string{
				"example.com/pkg": {"FooSuite": {"FuzzFooSuite_A", "FuzzFooSuite_B"}},
			}
			targets := gotestrunner.BuildSuiteTargets(compiled, suitesByPkg, dirsByPkg, fuzzFuncsByPkg, nil, nil, "")
			gotest.Len(it, targets, 1)
			gotest.Equal(it, "TestFooSuite", targets[0].SuiteName)
			gotest.Equal(it, []string{"FuzzFooSuite_A", "FuzzFooSuite_B"}, targets[0].FuzzFuncs)
		})

		w.It("leaves FuzzFuncs empty for a suite absent from fuzzFuncsByPkg", func(it *gotest.T) {
			suitesByPkg := map[string][]string{"example.com/pkg": {"FooSuite"}}
			fuzzFuncsByPkg := map[string]map[string][]string{
				"example.com/pkg": {"OtherSuite": {"FuzzOtherSuite_A"}},
			}
			targets := gotestrunner.BuildSuiteTargets(compiled, suitesByPkg, dirsByPkg, fuzzFuncsByPkg, nil, nil, "")
			gotest.Len(it, targets, 1)
			gotest.Empty(it, targets[0].FuzzFuncs)
		})

		w.It("tolerates a nil fuzzFuncsByPkg map", func(it *gotest.T) {
			suitesByPkg := map[string][]string{"example.com/pkg": {"FooSuite"}}
			targets := gotestrunner.BuildSuiteTargets(compiled, suitesByPkg, dirsByPkg, nil, nil, nil, "")
			gotest.Len(it, targets, 1)
			gotest.Empty(it, targets[0].FuzzFuncs)
		})
	})
}

func (s *SuiteCommandTestSuite) TestBuildBenchTargets(t *gotest.T) {
	compiled := []gotestrunner.CompileResult{
		{Package: "example.com/pkg", BinaryPath: "/tmp/pkg.test"},
	}
	dirsByPkg := map[string]string{"example.com/pkg": "/src/pkg"}

	t.When("building targets from benchesByPkg", func(w *gotest.T) {
		w.It("builds a Bench target per suite with the bare suite name", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/pkg": {"BenchTestSuite"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, nil, "", "")
			gotest.Len(it, targets, 1)
			gotest.True(it, targets[0].Bench)
			gotest.Equal(it, "BenchTestSuite", targets[0].SuiteName)
			gotest.Equal(it, "example.com/pkg", targets[0].Package)
			gotest.Equal(it, "/src/pkg", targets[0].Dir)
			gotest.Equal(it, "/tmp/pkg.test", targets[0].BinaryPath)
		})

		w.It("skips packages with no compiled binary", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/other": {"BenchTestSuite"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, nil, "", "")
			gotest.Empty(it, targets)
		})

		w.It("filters by userRunFilter (-run) matching Benchmark<Suite>", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/pkg": {"BenchTestSuite", "OtherSuite"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, nil, "^BenchmarkBenchTestSuite$", "")
			gotest.Len(it, targets, 1)
			gotest.Equal(it, "BenchTestSuite", targets[0].SuiteName)
		})

		w.It("filters by userBenchFilter (-bench) matching Benchmark<Suite>", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/pkg": {"BenchTestSuite", "OtherSuite"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, nil, "", "OtherSuite")
			gotest.Len(it, targets, 1)
			gotest.Equal(it, "OtherSuite", targets[0].SuiteName)
		})

		w.It("AND-composes -run and -bench when both are set", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/pkg": {"Foo", "Bar", "Baz"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, nil, "^Benchmark(Foo|Bar)$", "Foo")
			gotest.Len(it, targets, 1)
			gotest.Equal(it, "Foo", targets[0].SuiteName)
		})

		w.It("includes all targets when neither filter is set", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/pkg": {"Foo", "Bar", "Baz"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, nil, "", "")
			gotest.Len(it, targets, 3)
		})

		w.It("translates run flags to -test. prefixed form", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/pkg": {"BenchTestSuite"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, []string{"-benchtime=2x"}, "", "")
			gotest.Len(it, targets, 1)
			gotest.Contains(it, targets[0].RunFlags, "-test.benchtime=2x")
		})
	})

	t.When("the -bench pattern carries sub-benchmark segments", func(w *gotest.T) {
		w.It("selects the suite by the first segment and carries the full pattern as BenchFilter", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/pkg": {"CacheTestSuite", "OtherSuite"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, nil, "", "^BenchmarkCacheTestSuite$/^BenchmarkGetHit$")
			gotest.Len(it, targets, 1)
			gotest.Equal(it, "CacheTestSuite", targets[0].SuiteName)
			gotest.Equal(it, "^BenchmarkCacheTestSuite$/^BenchmarkGetHit$", targets[0].BenchFilter)
		})

		w.It("keeps only the alternation branches that match each suite's wrapper", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/pkg": {"Foo", "Bar"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, nil, "", "^BenchmarkFoo$/^BenchmarkA$|^BenchmarkBar$/^BenchmarkB$")
			gotest.Len(it, targets, 2)
			byName := map[string]string{}
			for i := range targets {
				byName[targets[i].SuiteName] = targets[i].BenchFilter
			}
			gotest.Equal(it, "^BenchmarkFoo$/^BenchmarkA$", byName["Foo"])
			gotest.Equal(it, "^BenchmarkBar$/^BenchmarkB$", byName["Bar"])
		})

		w.It("leaves BenchFilter empty for a suite-only pattern", func(it *gotest.T) {
			benchesByPkg := map[string][]string{"example.com/pkg": {"CacheTestSuite"}}
			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, nil, "", "^BenchmarkCacheTestSuite$")
			gotest.Len(it, targets, 1)
			gotest.Zero(it, targets[0].BenchFilter)
		})
	})

	t.When("building the bench subprocess command", func(w *gotest.T) {
		ctx := context.Background()

		w.It("forces the exact wrapper pattern without a BenchFilter", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "CacheTestSuite"},
				BinaryPath: "/tmp/pkg.test",
				Bench:      true,
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, `-test.bench=^BenchmarkCacheTestSuite$`)
		})

		w.It("hands a BenchFilter to the binary verbatim so go test scopes sub-benchmarks", func(it *gotest.T) {
			target := gotestrunner.SuiteTarget{
				SuiteSpec:   gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "CacheTestSuite"},
				BinaryPath:  "/tmp/pkg.test",
				Bench:       true,
				BenchFilter: "^BenchmarkCacheTestSuite$/^BenchmarkGetHit$",
			}
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, nil, false)
			gotest.Contains(it, cmd.Args, `-test.bench=^BenchmarkCacheTestSuite$/^BenchmarkGetHit$`)
			gotest.NotContains(it, cmd.Args, `-test.bench=^BenchmarkCacheTestSuite$`)
		})
	})
}

func (s *SuiteCommandTestSuite) TestBenchFilterNeverReachesTargetRunFlags(t *gotest.T) {
	t.When("a user -bench filter is present in raw run flags", func(w *gotest.T) {
		w.It("is extracted and stripped before BuildBenchTargets so it never leaks into RunFlags", func(it *gotest.T) {
			rawRunFlags := []string{"-bench=Foo", "-benchtime=2x"}
			userBenchFilter := gotestrunner.ExtractBenchFilter(rawRunFlags)
			runFlags := gotestrunner.StripBenchFilter(rawRunFlags)

			gotest.Equal(it, "Foo", userBenchFilter)
			gotest.NotContains(it, runFlags, "-bench=Foo")

			compiled := []gotestrunner.CompileResult{
				{Package: "example.com/pkg", BinaryPath: "/tmp/pkg.test"},
			}
			benchesByPkg := map[string][]string{"example.com/pkg": {"Foo"}}
			dirsByPkg := map[string]string{"example.com/pkg": "/src/pkg"}

			targets := gotestrunner.BuildBenchTargets(compiled, benchesByPkg, dirsByPkg, runFlags, "", userBenchFilter)
			gotest.Len(it, targets, 1)
			for _, f := range targets[0].RunFlags {
				leaked := f == "-test.bench" || strings.HasPrefix(f, "-test.bench=")
				gotest.False(it, leaked, "user -bench leaked into target RunFlags: %v", targets[0].RunFlags)
			}
			gotest.Contains(it, targets[0].RunFlags, "-test.benchtime=2x")
		})
	})
}
