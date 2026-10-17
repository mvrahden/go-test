package main_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// LintFixTestSuite runs the built CLI's lint -fix, which applies fixes in
// memory until none is left, writes them and reports what is left, and
// lint -diff, which prints what -fix would write instead.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type LintFixTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *LintFixTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *LintFixTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

// lintFixProbe has one fixable finding (True) and one without a fix (Helper).
const lintFixProbe = `package lintfix

import "github.com/mvrahden/go-test/pkg/gotest"

type ProbeTestSuite struct{}

func (s *ProbeTestSuite) TestProbe(t *gotest.T) {
	t.T().Helper()
	gotest.True(t, 1 == 1)
}
`

// lintFixOverlapProbe has fixes that overlap: the fail-guard rewrite spans
// the t-escape one, so the driver applies one per pass. Helper has no fix.
const lintFixOverlapProbe = `package lintfix

import "github.com/mvrahden/go-test/pkg/gotest"

type ProbeTestSuite struct{}

func (s *ProbeTestSuite) TestProbe(t *gotest.T) {
	x := 3
	if x != 3 {
		gotest.Fail(t.T(), "bad")
	}
	t.T().Helper()
	gotest.True(t, 1 == 1)
}
`

// helperFinding is the plain-text finding Helper leaves, at line.
func helperFinding(line string) string {
	return `^\S*probe_test\.go:` + line + `:2: never call Helper .+\n$`
}

// runLintIn runs the binary in dir with extra env entries and returns
// stdout, stderr and exit code.
func (s *LintFixTestSuite) runLintIn(t *gotest.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	cmd := exec.Command(s.cli.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "PWD="+dir), env...)
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	err := cmd.Run()
	var exitErr *exec.ExitError
	gotest.True(t, err == nil || errors.As(err, &exitErr), "running gotest binary: %v\n%s", err, errs.String())
	return out.String(), errs.String(), cmd.ProcessState.ExitCode()
}

func (s *LintFixTestSuite) stageProbe(t *gotest.T, src string) (dir, file string) {
	dir = t.TempDir()
	gotest.NoError(t, testkit.WriteModule(s.cli.repoRoot, dir, "lintfix"))
	file = filepath.Join(dir, "probe_test.go")
	gotest.NoError(t, os.WriteFile(file, []byte(src), 0o600))
	return dir, file
}

func readLintFixFile(t *gotest.T, file string) string {
	src, err := os.ReadFile(file)
	gotest.NoError(t, err)
	return string(src)
}

// goShim puts a `go` ahead of the real one on PATH that runs script before
// it, and returns the PATH entry. Unix only: the shim is a shell script.
func goShim(t *gotest.T, script string) string {
	if runtime.GOOS == "windows" {
		t.Skipf("the go shim is a shell script")
	}
	realGo, err := exec.LookPath("go")
	gotest.NoError(t, err)
	dir := t.TempDir()
	body := "#!/bin/sh\n" + script + "\nexec '" + realGo + "' \"$@\"\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go"), []byte(body), 0o700)) //nolint:gosec // G306: the shim must be executable
	return "PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func (s *LintFixTestSuite) TestFix(t *gotest.T) {
	t.When("a finding has no fix", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixProbe)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")
		src := readLintFixFile(w, file)

		w.It("applies the fixable rewrite", func(it *gotest.T) {
			gotest.Contains(it, src, "gotest.Equal(t, 1, 1)")
		})
		w.It("reports the finding it could not fix", func(it *gotest.T) {
			gotest.Regexp(it, helperFinding("8"), stderr)
			gotest.Empty(it, stdout)
		})
		w.It("exits 3", func(it *gotest.T) {
			gotest.Equal(it, 3, code, stderr)
		})
	})

	t.When("every finding has a fix", func(w *gotest.T) {
		dir, file := s.stageProbe(w, strings.Replace(lintFixProbe, "\tt.T().Helper()\n", "", 1))
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")

		w.It("exits 0 and says nothing", func(it *gotest.T) {
			gotest.Equal(it, 0, code, stderr)
			gotest.Empty(it, stderr)
			gotest.Empty(it, stdout)
		})
		w.It("applies the rewrite", func(it *gotest.T) {
			gotest.Contains(it, readLintFixFile(it, file), "gotest.Equal(t, 1, 1)")
		})
	})

	t.When("fixes overlap", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixOverlapProbe)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")
		src := readLintFixFile(w, file)

		w.It("applies every fix in one run", func(it *gotest.T) {
			gotest.Contains(it, src, `gotest.Equal(t, 3, x, "bad")`)
			gotest.Contains(it, src, "gotest.Equal(t, 1, 1)")
			gotest.NotContains(it, src, "t.T(), ")
		})
		w.It("reports only the finding without a fix and exits 3", func(it *gotest.T) {
			gotest.Regexp(it, helperFinding("10"), stderr)
			gotest.Empty(it, stdout)
			gotest.Equal(it, 3, code, stderr)
		})
	})

	t.When("fixes overlap and every finding has one", func(w *gotest.T) {
		dir, _ := s.stageProbe(w, strings.Replace(lintFixOverlapProbe, "\tt.T().Helper()\n", "", 1))
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")

		w.It("exits 0 and says nothing", func(it *gotest.T) {
			gotest.Equal(it, 0, code, stderr)
			gotest.Empty(it, stderr)
			gotest.Empty(it, stdout)
		})
	})

	t.When("the flag follows the pattern", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixProbe)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "./...", "-fix")

		w.It("still fixes and reports what is left", func(it *gotest.T) {
			gotest.Contains(it, readLintFixFile(it, file), "gotest.Equal(t, 1, 1)")
			gotest.Regexp(it, helperFinding("8"), stderr)
			gotest.Empty(it, stdout)
			gotest.Equal(it, 3, code, stderr)
		})
	})
}

func (s *LintFixTestSuite) TestLoads(t *gotest.T) {
	// The shim logs every package load: go/packages runs one go list -json
	// per load, in lint and in the child driver alike.
	loads := func(t *gotest.T, log string) int {
		data, err := os.ReadFile(log)
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		gotest.NoError(t, err)
		return strings.Count(string(data), "list -e -json=")
	}

	t.When("no finding has a fix", func(w *gotest.T) {
		log := filepath.Join(w.TempDir(), "loads")
		path := goShim(w, `echo "$*" >> '`+log+`'`)
		dir, file := s.stageProbe(w, strings.Replace(lintFixProbe, "\tgotest.True(t, 1 == 1)\n", "", 1))
		stdout, stderr, code := s.runLintIn(w, dir, []string{path}, "lint", "-fix", "./...")

		w.It("loads the targets once and starts no driver", func(it *gotest.T) {
			gotest.Equal(it, 1, loads(it, log))
		})
		w.It("reports the finding and leaves the file alone", func(it *gotest.T) {
			gotest.Regexp(it, helperFinding("8"), stderr)
			gotest.Empty(it, stdout)
			gotest.Equal(it, 3, code, stderr)
			gotest.NotContains(it, readLintFixFile(it, file), "Equal")
		})
	})

	t.When("the targets have no findings", func(w *gotest.T) {
		log := filepath.Join(w.TempDir(), "loads")
		path := goShim(w, `echo "$*" >> '`+log+`'`)
		dir, _ := s.stageProbe(w, strings.Replace(lintFixProbe, "\tt.T().Helper()\n\tgotest.True(t, 1 == 1)\n", "", 1))
		_, stderr, code := s.runLintIn(w, dir, []string{path}, "lint", "-fix", "-test=false", "-c", "2", "./...")

		w.It("loads them once, even with flags the analyzer does not own", func(it *gotest.T) {
			gotest.Equal(it, 0, code, stderr)
			gotest.Equal(it, 1, loads(it, log))
		})
	})
}

func (s *LintFixTestSuite) TestOutputModes(t *gotest.T) {
	t.When("-json is set", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixOverlapProbe)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "-json", "./...")

		w.It("prints only the findings left, as JSON on stdout", func(it *gotest.T) {
			gotest.True(it, json.Valid([]byte(stdout)), stdout)
			gotest.Equal(it, 1, strings.Count(stdout, `"message"`), stdout)
			gotest.Contains(it, stdout, "never call Helper")
			gotest.Empty(it, stderr)
		})
		w.It("exits 0, as -json does without -fix", func(it *gotest.T) {
			gotest.Equal(it, 0, code, stderr)
		})
		w.It("applies the fixes", func(it *gotest.T) {
			gotest.Contains(it, readLintFixFile(it, file), `gotest.Equal(t, 3, x, "bad")`)
		})
	})

	t.When("--github is set", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixOverlapProbe)
		summary := filepath.Join(w.TempDir(), "summary.md")
		stdout, stderr, code := s.runLintIn(w, dir, []string{"GITHUB_STEP_SUMMARY=" + summary}, "lint", "-fix", "--github", "./...")

		w.It("annotates only the finding left", func(it *gotest.T) {
			gotest.Regexp(it, `^::error file=probe_test\.go,line=10,col=2,title=t-escape::never call Helper .+\n$`, stdout)
			gotest.Empty(it, stderr)
			gotest.Equal(it, 3, code, stderr)
		})
		w.It("applies the fixes", func(it *gotest.T) {
			gotest.Contains(it, readLintFixFile(it, file), `gotest.Equal(t, 3, x, "bad")`)
		})
	})

	t.When("-diff and -json are set", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixProbe)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-diff", "-json", "./...")

		w.It("exits 2, since both print on stdout", func(it *gotest.T) {
			gotest.Equal(it, 2, code, stderr)
			gotest.Equal(it, "FAIL: -diff and -json both print on stdout; use one\n", stderr)
			gotest.Empty(it, stdout)
		})
		w.It("writes nothing", func(it *gotest.T) {
			gotest.Equal(it, lintFixProbe, readLintFixFile(it, file))
		})
	})

	t.When("-fix is set with a flag of the go/analysis driver", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixProbe)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "-debug=v", "./...")

		w.It("exits 2 and names the flag", func(it *gotest.T) {
			gotest.Equal(it, 2, code, stderr)
			gotest.Equal(it, "FAIL: -debug=v is a go/analysis driver flag; -fix and -diff do not take it\n", stderr)
			gotest.Empty(it, stdout)
		})
		w.It("writes nothing", func(it *gotest.T) {
			gotest.Equal(it, lintFixProbe, readLintFixFile(it, file))
		})
	})
}

// lintFixOverlapDiff is what lint -diff prints for lintFixOverlapProbe: the
// file as -fix leaves it, every pass applied.
const lintFixOverlapDiff = `--- a/probe_test.go
+++ b/probe_test.go
@@ -6,9 +6,7 @@
 
 func (s *ProbeTestSuite) TestProbe(t *gotest.T) {
 	x := 3
-	if x != 3 {
-		gotest.Fail(t.T(), "bad")
-	}
+	gotest.Equal(t, 3, x, "bad")
 	t.T().Helper()
-	gotest.True(t, 1 == 1)
+	gotest.Equal(t, 1, 1)
 }
`

func (s *LintFixTestSuite) TestDiff(t *gotest.T) {
	t.When("fixes overlap", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixOverlapProbe)
		before := listDir(w, dir)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "-diff", "./...")

		w.It("prints every pass of fixes as one diff on stdout", func(it *gotest.T) {
			gotest.Equal(it, lintFixOverlapDiff, stdout)
		})
		w.It("reports the finding -fix would leave, where -fix leaves it, and exits 3", func(it *gotest.T) {
			gotest.Regexp(it, helperFinding("10"), stderr)
			gotest.Equal(it, 3, code, stderr)
		})
		w.It("writes nothing", func(it *gotest.T) {
			gotest.Equal(it, lintFixOverlapProbe, readLintFixFile(it, file))
			gotest.Equal(it, before, listDir(it, dir))
		})
	})

	t.When("-c asks for source lines", func(w *gotest.T) {
		dir, _ := s.stageProbe(w, lintFixOverlapProbe)
		_, stderr, code := s.runLintIn(w, dir, nil, "lint", "-diff", "-c", "0", "./...")

		w.It("shows them as -fix would leave them", func(it *gotest.T) {
			gotest.Regexp(it, `probe_test\.go:10:2: never call Helper .+\n10\t\tt\.T\(\)\.Helper\(\)\n$`, stderr)
			gotest.Equal(it, 3, code, stderr)
		})
	})

	t.When("-diff is given without -fix", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixOverlapProbe)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-diff", "./...")

		w.It("previews -fix all the same", func(it *gotest.T) {
			gotest.Equal(it, lintFixOverlapDiff, stdout)
			gotest.Regexp(it, helperFinding("10"), stderr)
			gotest.Equal(it, 3, code, stderr)
			gotest.Equal(it, lintFixOverlapProbe, readLintFixFile(it, file))
		})
	})

	t.When("every finding has a fix", func(w *gotest.T) {
		dir, file := s.stageProbe(w, strings.Replace(lintFixOverlapProbe, "\tt.T().Helper()\n", "", 1))
		stdout, stderr, code := s.runLintIn(w, dir, []string{"GITHUB_ACTIONS=true"}, "lint", "-diff", "./...")

		w.It("prints only the diff, even inside GitHub Actions, and exits 0", func(it *gotest.T) {
			gotest.Regexp(it, `^--- a/probe_test\.go\n\+\+\+ b/probe_test\.go\n@@ `, stdout)
			gotest.NotContains(it, stdout, "::error")
			gotest.Empty(it, stderr)
			gotest.Equal(it, 0, code, stderr)
		})
		w.It("writes nothing", func(it *gotest.T) {
			gotest.Equal(it, strings.Replace(lintFixOverlapProbe, "\tt.T().Helper()\n", "", 1), readLintFixFile(it, file))
		})
	})

	t.When("no finding has a fix", func(w *gotest.T) {
		dir, _ := s.stageProbe(w, strings.Replace(lintFixProbe, "\tgotest.True(t, 1 == 1)\n", "", 1))
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-diff", "./...")

		w.It("prints no diff, reports the finding and exits 3", func(it *gotest.T) {
			gotest.Empty(it, stdout)
			gotest.Regexp(it, helperFinding("8"), stderr)
			gotest.Equal(it, 3, code, stderr)
		})
	})

	t.When("a target does not compile", func(w *gotest.T) {
		dir, _ := s.stageProbe(w, lintFixProbe)
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "broken_test.go"), []byte("package lintfix\n\nvar x int = \"nope\"\n"), 0o600))
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-diff", "./...")

		w.It("prints no diff and exits 1", func(it *gotest.T) {
			gotest.Empty(it, stdout)
			gotest.Regexp(it, `^FAIL: cannot lint uncompilable packages`, stderr)
			gotest.Equal(it, 1, code, stderr)
		})
	})
}

// listDir names dir's entries with their sizes and modification times.
func listDir(t *gotest.T, dir string) []string {
	entries, err := os.ReadDir(dir)
	gotest.NoError(t, err)
	var out []string
	for _, e := range entries {
		info, err := e.Info()
		gotest.NoError(t, err)
		out = append(out, fmt.Sprintf("%s %d %s", e.Name(), info.Size(), info.ModTime()))
	}
	return out
}

func (s *LintFixTestSuite) TestSkips(t *gotest.T) {
	// The overlap probe gives the child driver work, so a skip it does not
	// receive shows as a True rewritten anyway.
	t.When("lint.skip in .gotest.yml disables a rule", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixOverlapProbe)
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, ".gotest.yml"), []byte("lint:\n  skip: [assertion-simplify]\n"), 0o600))
		_, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")
		src := readLintFixFile(w, file)

		w.It("leaves that rule's findings unfixed and unreported", func(it *gotest.T) {
			gotest.Contains(it, src, "gotest.True(t, 1 == 1)")
			gotest.Contains(it, src, `gotest.Equal(t, 3, x, "bad")`)
			gotest.Regexp(it, helperFinding("10"), stderr)
			gotest.Equal(it, 3, code, stderr)
		})
	})

	t.When("a -skip flag disables a rule", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixOverlapProbe)
		_, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "-skip-assertion-simplify", "./...")
		src := readLintFixFile(w, file)

		w.It("leaves that rule's findings unfixed and unreported", func(it *gotest.T) {
			gotest.Contains(it, src, "gotest.True(t, 1 == 1)")
			gotest.Contains(it, src, `gotest.Equal(t, 3, x, "bad")`)
			gotest.Regexp(it, helperFinding("10"), stderr)
			gotest.Equal(it, 3, code, stderr)
		})
	})
}

// stageWorkspace writes a workspace of modules one and two, each with
// lintFixProbe, and returns its root and the probe files.
func (s *LintFixTestSuite) stageWorkspace(t *gotest.T) (root string, files []string) {
	root = t.TempDir()
	for _, mod := range []string{"one", "two"} {
		dir := filepath.Join(root, mod)
		gotest.NoError(t, os.Mkdir(dir, 0o700))
		gotest.NoError(t, testkit.WriteModule(s.cli.repoRoot, dir, mod))
		// The workspace at root holds both, from either.
		gotest.NoError(t, os.Remove(filepath.Join(dir, "go.work")))
		file := filepath.Join(dir, "probe_test.go")
		gotest.NoError(t, os.WriteFile(file, []byte(strings.Replace(lintFixProbe, "package lintfix", "package "+mod, 1)), 0o600))
		files = append(files, file)
	}
	work := "go 1.26.0\n\nuse (\n\t./one\n\t./two\n\t" + s.cli.repoRoot + "\n)\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(root, "go.work"), []byte(work), 0o600))
	return root, files
}

// gitApply applies patch with git apply and args in dir, inside a fresh
// repository at root.
func gitApply(t *gotest.T, root, dir, patch string, args ...string) (string, error) {
	git := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", args...) //nolint:gosec // G204: fixed git subcommands
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(patch)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := git(root, "init", "-q")
	gotest.NoError(t, err, out)
	return git(dir, append([]string{"apply"}, args...)...)
}

func (s *LintFixTestSuite) TestTargets(t *gotest.T) {
	t.When("patterns name modules of a workspace", func(w *gotest.T) {
		root, files := s.stageWorkspace(w)

		stdout, stderr, code := s.runLintIn(w, root, nil, "lint", "-fix", "./one/...", "./two/...")

		w.It("fixes both", func(it *gotest.T) {
			for _, file := range files {
				gotest.Contains(it, readLintFixFile(it, file), "gotest.Equal(t, 1, 1)")
			}
		})
		w.It("reports what is left in both and exits 3", func(it *gotest.T) {
			gotest.Regexp(it, `^\S*one/probe_test\.go:8:2: never call Helper .+\n\S*two/probe_test\.go:8:2: never call Helper .+\n$`, filepath.ToSlash(stderr))
			gotest.Empty(it, stdout)
			gotest.Equal(it, 3, code, stderr)
		})
	})

	t.When("-diff runs in one module of a workspace and names its sibling", func(w *gotest.T) {
		root, files := s.stageWorkspace(w)
		one := filepath.Join(root, "one")
		stdout, stderr, code := s.runLintIn(w, one, nil, "lint", "-diff", "./...", "../two/...")
		_, plainErr := gitApply(w, root, one, stdout)
		out, err := gitApply(w, root, one, stdout, "--unsafe-paths")

		w.It("names each file relative to where lint ran, with a/ and b/", func(it *gotest.T) {
			gotest.Regexp(it, `^--- a/probe_test\.go\n\+\+\+ b/probe_test\.go\n`, stdout)
			gotest.Contains(it, stdout, "\n--- a/../two/probe_test.go\n+++ b/../two/probe_test.go\n")
		})
		w.It("says git apply needs --unsafe-paths, before the findings", func(it *gotest.T) {
			gotest.Regexp(it, `^lint: the diff names files outside the current directory; apply it with git apply --unsafe-paths\n`, stderr)
			gotest.Equal(it, 3, code, stderr)
		})
		w.It("applies with git apply --unsafe-paths there, and only with it", func(it *gotest.T) {
			gotest.Error(it, plainErr)
			gotest.NoError(it, err, out)
			for _, file := range files {
				gotest.Contains(it, readLintFixFile(it, file), "gotest.Equal(t, 1, 1)")
			}
		})
	})

	t.When("-diff runs in a subdirectory of a repository", func(w *gotest.T) {
		root, files := s.stageWorkspace(w)
		two := filepath.Join(root, "two")
		stdout, stderr, code := s.runLintIn(w, two, nil, "lint", "-diff", "./...")
		out, err := gitApply(w, root, two, stdout)

		w.It("applies with git apply there", func(it *gotest.T) {
			gotest.Equal(it, 3, code, stderr)
			gotest.NotContains(it, stderr, "--unsafe-paths")
			gotest.NoError(it, err, out)
			gotest.Equal(it, lintFixProbe, strings.Replace(readLintFixFile(it, files[0]), "package one", "package lintfix", 1))
			gotest.Contains(it, readLintFixFile(it, files[1]), "gotest.Equal(t, 1, 1)")
		})
	})

	t.When("-diff covers a file that is a symlink", func(w *gotest.T) {
		if runtime.GOOS == "windows" {
			w.Skipf("symlinks need privileges on Windows")
		}
		dir, _ := s.stageProbe(w, lintFixProbe)
		gotest.NoError(w, os.Mkdir(filepath.Join(dir, "real"), 0o700))
		real := filepath.Join(dir, "real", "second.go.txt")
		gotest.NoError(w, os.WriteFile(real, []byte(lintFixSecondProbe), 0o600))
		link := filepath.Join(dir, "second_test.go")
		gotest.NoError(w, os.Symlink(filepath.Join("real", "second.go.txt"), link))
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-diff", "./...")
		out, err := gitApply(w, dir, dir, stdout)
		info, lerr := os.Lstat(link)

		w.It("names the file the link leads to, which git apply writes", func(it *gotest.T) {
			gotest.Equal(it, 3, code, stderr)
			gotest.Contains(it, stdout, "\n--- a/real/second.go.txt\n+++ b/real/second.go.txt\n")
			gotest.NoError(it, err, out)
			gotest.Contains(it, readLintFixFile(it, real), "gotest.Equal(t, 2, 2)")
			gotest.NoError(it, lerr)
			gotest.NotZero(it, info.Mode()&os.ModeSymlink)
		})
	})

	t.When("a target does not compile", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixProbe)
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "broken_test.go"), []byte("package lintfix\n\nvar x int = \"nope\"\n"), 0o600))
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")

		w.It("exits 1 and names it", func(it *gotest.T) {
			gotest.Equal(it, 1, code, stderr)
			gotest.Regexp(it, `^FAIL: cannot lint uncompilable packages`, stderr)
			gotest.Empty(it, stdout)
		})
		w.It("touches no file", func(it *gotest.T) {
			gotest.Equal(it, lintFixProbe, readLintFixFile(it, file))
		})
	})
}

func (s *LintFixTestSuite) TestTags(t *gotest.T) {
	// The tagged file has a fixable finding and one -fix leaves.
	stage := func(t *gotest.T) (dir, tagged string) {
		dir, _ = s.stageProbe(t, strings.Replace(lintFixProbe, "\tt.T().Helper()\n\tgotest.True(t, 1 == 1)\n", "", 1))
		tagged = filepath.Join(dir, "tagged_test.go")
		gotest.NoError(t, os.WriteFile(tagged, []byte("//go:build integration\n\n"+strings.Replace(lintFixSecondProbe, "\tgotest.True", "\tt.T().Helper()\n\tgotest.True", 1)), 0o600))
		return dir, tagged
	}
	taggedFinding := `^\S*tagged_test\.go:8:2: never call Helper .+\n$`

	t.When("-tags names the tag a file needs", func(w *gotest.T) {
		dir, _ := stage(w)
		_, plain, plainCode := s.runLintIn(w, dir, nil, "lint", "-tags=integration", "./...")
		_, driver, driverCode := s.runLintIn(w, dir, nil, "lint", "-tags", "integration", "-debug=", "./...")
		_, without, withoutCode := s.runLintIn(w, dir, nil, "lint", "./...")

		w.It("lints the file, in process and through the go/analysis driver", func(it *gotest.T) {
			gotest.Regexp(it, `tagged_test\.go:8:2: never call Helper .+\n\S*tagged_test\.go:9:2: use Equal`, plain)
			gotest.Equal(it, 3, plainCode, plain)
			gotest.Regexp(it, `tagged_test\.go:8:2: never call Helper`, driver)
			gotest.Equal(it, 3, driverCode, driver)
		})
		w.It("skips the file without it", func(it *gotest.T) {
			gotest.Empty(it, without)
			gotest.Equal(it, 0, withoutCode, without)
		})
	})

	t.When("-fix is given -tags", func(w *gotest.T) {
		dir, tagged := stage(w)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "-tags=integration", "./...")

		w.It("fixes the file and reports, from the fixed sources, what is left in it", func(it *gotest.T) {
			gotest.Contains(it, readLintFixFile(it, tagged), "gotest.Equal(t, 2, 2)")
			gotest.Regexp(it, taggedFinding, stderr)
			gotest.Empty(it, stdout)
			gotest.Equal(it, 3, code, stderr)
		})
	})
}

// lintFixSecondProbe adds a second file with a fixable finding.
const lintFixSecondProbe = `package lintfix

import "github.com/mvrahden/go-test/pkg/gotest"

func (s *ProbeTestSuite) TestSecond(t *gotest.T) {
	gotest.True(t, 2 == 2)
}
`

func (s *LintFixTestSuite) TestWrites(t *gotest.T) {
	t.When("one of the fixed files cannot be written", func(w *gotest.T) {
		if runtime.GOOS == "windows" {
			w.Skipf("read-only files are still renamed over on Windows' permission model")
		}
		dir, file := s.stageProbe(w, lintFixProbe)
		second := filepath.Join(dir, "second_test.go")
		gotest.NoError(w, os.WriteFile(second, []byte(lintFixSecondProbe), 0o400))
		before := listDir(w, dir)
		stdout, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")

		w.It("exits 1 and says nothing was written", func(it *gotest.T) {
			gotest.Regexp(it, `^FAIL: writing fixes: open \S*second_test\.go: permission denied; nothing was written\n$`, stderr)
			gotest.Empty(it, stdout)
			gotest.Equal(it, 1, code, stderr)
		})
		w.It("writes none of them and leaves no temporary file", func(it *gotest.T) {
			gotest.Equal(it, lintFixProbe, readLintFixFile(it, file))
			gotest.Equal(it, before, listDir(it, dir))
		})
	})

	t.When("a fixed file changes on disk while lint fixes it", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixProbe)
		// The shim edits the file at the second load, of the fixed sources.
		count := filepath.Join(w.TempDir(), "loads")
		path := goShim(w, `case "$*" in *-json=*) echo >> '`+count+`'; if [ "$(wc -l < '`+count+`')" -eq 2 ]; then echo '// edited' >> '`+file+`'; fi;; esac`)
		stdout, stderr, code := s.runLintIn(w, dir, []string{path}, "lint", "-fix", "./...")

		w.It("exits 1, says nothing was written and keeps the edit", func(it *gotest.T) {
			gotest.Regexp(it, `^FAIL: writing fixes: \S*probe_test\.go changed since it was read; nothing was written\n$`, stderr)
			gotest.Empty(it, stdout)
			gotest.Equal(it, 1, code, stderr)
			gotest.Equal(it, lintFixProbe+"// edited\n", readLintFixFile(it, file))
		})
	})

	t.When("the fixed files are a symlink and a file with its own mode", func(w *gotest.T) {
		if runtime.GOOS == "windows" {
			w.Skipf("symlinks and permission bits are POSIX")
		}
		dir, file := s.stageProbe(w, lintFixProbe)
		gotest.NoError(w, os.Chmod(file, 0o640))
		real := filepath.Join(w.TempDir(), "second.go.txt")
		gotest.NoError(w, os.WriteFile(real, []byte(lintFixSecondProbe), 0o600))
		link := filepath.Join(dir, "second_test.go")
		gotest.NoError(w, os.Symlink(real, link))
		_, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")
		info, statErr := os.Stat(file)
		linkInfo, lstatErr := os.Lstat(link)

		w.It("keeps the mode", func(it *gotest.T) {
			gotest.Equal(it, 3, code, stderr)
			gotest.NoError(it, statErr)
			gotest.Equal(it, os.FileMode(0o640), info.Mode().Perm())
			gotest.Contains(it, readLintFixFile(it, file), "gotest.Equal(t, 1, 1)")
		})
		w.It("fixes the file the link names and keeps the link", func(it *gotest.T) {
			gotest.NoError(it, lstatErr)
			gotest.NotZero(it, linkInfo.Mode()&os.ModeSymlink)
			gotest.Contains(it, readLintFixFile(it, real), "gotest.Equal(t, 2, 2)")
		})
	})

	t.When("a fixed file has a second hard link", func(w *gotest.T) {
		dir, file := s.stageProbe(w, lintFixProbe)
		other := filepath.Join(w.TempDir(), "probe.go.txt")
		gotest.NoError(w, os.Link(file, other))
		_, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")

		w.It("fixes the file both names share", func(it *gotest.T) {
			gotest.Equal(it, 3, code, stderr)
			gotest.Contains(it, readLintFixFile(it, file), "gotest.Equal(t, 1, 1)")
			gotest.Equal(it, readLintFixFile(it, file), readLintFixFile(it, other))
		})
	})

	t.When("a fixed file is writable but its directory is not", func(w *gotest.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			w.Skipf("POSIX directory permissions as a non-root user")
		}
		dir, file := s.stageProbe(w, lintFixProbe)
		gotest.NoError(w, os.Chmod(dir, 0o500))
		_, stderr, code := s.runLintIn(w, dir, nil, "lint", "-fix", "./...")
		gotest.NoError(w, os.Chmod(dir, 0o700))

		w.It("fixes it in place", func(it *gotest.T) {
			gotest.Equal(it, 3, code, stderr)
			gotest.Contains(it, readLintFixFile(it, file), "gotest.Equal(t, 1, 1)")
		})
	})
}

// leftovers names the entries left in dir.
func leftovers(t *gotest.R, dir string) []string {
	entries, err := os.ReadDir(dir)
	gotest.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func (s *LintFixTestSuite) TestInterrupt(t *gotest.T) {
	t.When("lint is stopped while it fixes", func(w *gotest.T) {
		// The shim holds the second load, of the fixed sources, so the
		// signal lands mid-fix.
		tmp := w.TempDir()
		count, marker := filepath.Join(tmp, "loads"), filepath.Join(tmp, "driver")
		path := goShim(w, `case "$*" in *-json=*) echo >> '`+count+`'; if [ "$(wc -l < '`+count+`')" -eq 2 ]; then : > '`+marker+`'; sleep 5; fi;; esac`)
		dir, file := s.stageProbe(w, lintFixProbe)
		tmpdir := w.TempDir()

		cmd := exec.Command(s.cli.binary, "lint", "-fix", "./...") //nolint:gosec // G204: controlled binary with fixed args
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PWD="+dir, path, "TMPDIR="+tmpdir)
		var errs bytes.Buffer
		cmd.Stderr = &errs
		gotest.NoError(w, cmd.Start())
		gotest.Eventually(w, 30*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
			_, err := os.Stat(marker)
			gotest.NoError(poll, err)
		})
		// The signal goes to lint alone, as from an editor's cancel.
		gotest.NoError(w, cmd.Process.Signal(syscall.SIGTERM))
		_ = cmd.Wait()
		code := cmd.ProcessState.ExitCode()

		w.It("exits 130", func(it *gotest.T) {
			gotest.Equal(it, 130, code, errs.String())
			gotest.Equal(it, "lint: interrupted; nothing was written\n", errs.String())
		})
		w.It("writes nothing, then or later, and leaves no temporary file", func(it *gotest.T) {
			gotest.Consistently(it, 7*time.Second, 250*time.Millisecond, func(poll *gotest.R) {
				src, err := os.ReadFile(file)
				gotest.NoError(poll, err)
				gotest.Equal(poll, lintFixProbe, string(src))
				gotest.Empty(poll, leftovers(poll, tmpdir))
			})
		})
	})

	t.When("lint is stopped twice while it fixes", func(w *gotest.T) {
		tmp := w.TempDir()
		count, marker := filepath.Join(tmp, "loads"), filepath.Join(tmp, "driver")
		path := goShim(w, `case "$*" in *-json=*) echo >> '`+count+`'; if [ "$(wc -l < '`+count+`')" -eq 2 ]; then : > '`+marker+`'; sleep 5; fi;; esac`)
		dir, file := s.stageProbe(w, lintFixProbe)
		tmpdir := w.TempDir()

		cmd := exec.Command(s.cli.binary, "lint", "-fix", "./...") //nolint:gosec // G204: controlled binary with fixed args
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PWD="+dir, path, "TMPDIR="+tmpdir)
		var errs bytes.Buffer
		cmd.Stderr = &errs
		gotest.NoError(w, cmd.Start())
		exited := make(chan struct{})
		go func() { _ = cmd.Wait(); close(exited) }()
		gotest.Eventually(w, 30*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
			_, err := os.Stat(marker)
			gotest.NoError(poll, err)
		})
		// Signals repeat until lint ends: the first stops the load, which
		// the shim holds for 5s; a later one ends lint at once.
		gotest.Eventually(w, 4*time.Second, 200*time.Millisecond, func(poll *gotest.R) {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-exited:
			default:
				gotest.Fail(poll, "lint still runs")
			}
		})
		code := cmd.ProcessState.ExitCode()

		w.It("exits 130 and says nothing was written", func(it *gotest.T) {
			gotest.Equal(it, 130, code, errs.String())
			gotest.Equal(it, "lint: interrupted again; nothing was written\n", errs.String())
		})
		w.It("writes nothing, then or later, and leaves no temporary file", func(it *gotest.T) {
			gotest.Consistently(it, 7*time.Second, 250*time.Millisecond, func(poll *gotest.R) {
				src, err := os.ReadFile(file)
				gotest.NoError(poll, err)
				gotest.Equal(poll, lintFixProbe, string(src))
				gotest.Empty(poll, leftovers(poll, tmpdir))
			})
		})
	})

	t.When("lint is stopped while go list builds export data", func(w *gotest.T) {
		if runtime.GOOS == "windows" {
			w.Skipf("SIGTERM and TMPDIR are POSIX")
		}
		// A cold cache and one build at a time keep go list building for
		// longer than its interrupt grace, so it is killed mid-build.
		dir, _ := s.stageProbe(w, lintFixProbe)
		tmpdir := w.TempDir()
		cmd := exec.Command(s.cli.binary, "lint", "-fix", "./...") //nolint:gosec // G204: controlled binary with fixed args
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PWD="+dir, "TMPDIR="+tmpdir, "GOCACHE="+w.TempDir(), "GOFLAGS=-p=1")
		var errs bytes.Buffer
		cmd.Stderr = &errs
		gotest.NoError(w, cmd.Start())
		gotest.Eventually(w, 30*time.Second, 50*time.Millisecond, func(poll *gotest.R) {
			builds, err := filepath.Glob(filepath.Join(tmpdir, "*", "go-build*"))
			gotest.NoError(poll, err)
			flat, err := filepath.Glob(filepath.Join(tmpdir, "go-build*"))
			gotest.NoError(poll, err)
			gotest.NotEmpty(poll, append(builds, flat...))
		})
		gotest.NoError(w, cmd.Process.Signal(syscall.SIGTERM))
		_ = cmd.Wait()

		w.It("exits 130 and leaves no work directory of go list behind", func(it *gotest.T) {
			gotest.Equal(it, 130, cmd.ProcessState.ExitCode(), errs.String())
			gotest.Consistently(it, 2*time.Second, 250*time.Millisecond, func(poll *gotest.R) {
				gotest.Empty(poll, leftovers(poll, tmpdir))
			})
		})
	})
}

func (s *LintFixTestSuite) TestLoadFailure(t *gotest.T) {
	t.When("the load of the fixed sources fails", func(w *gotest.T) {
		// The shim fails the second load, of the fixed sources.
		count := filepath.Join(w.TempDir(), "loads")
		path := goShim(w, `case "$*" in *-json=*) echo >> '`+count+`'; if [ "$(wc -l < '`+count+`')" -eq 2 ]; then exit 1; fi;; esac`)
		dir, file := s.stageProbe(w, lintFixProbe)
		stdout, stderr, code := s.runLintIn(w, dir, []string{path}, "lint", "-fix", "./...")

		w.It("exits 1 and says why", func(it *gotest.T) {
			gotest.Equal(it, 1, code, stderr)
			// go/packages turns a failed go list into an empty load.
			gotest.Equal(it, "FAIL: after applying fixes: \"./...\" matched no packages\n", stderr)
			gotest.Empty(it, stdout)
		})
		w.It("touches no file", func(it *gotest.T) {
			gotest.Equal(it, lintFixProbe, readLintFixFile(it, file))
		})
	})
}
