package skillevals_test

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SkillCommandsTestSuite: every sh block of SKILL.md runs as written in a
// consumer module.
//
// Sequential: the methods stage under one fixed root and each builds the CLI.
type SkillCommandsTestSuite struct {
	dir string // the staged consumer fixture of the running method
}

func (s *SkillCommandsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 2 * time.Minute}
}

func (s *SkillCommandsTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the skill's sh blocks need a POSIX shell"
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return "bash is not on PATH"
	}
	return ""
}

// BeforeEach stages a copy of the consumer fixture at a fixed path, so Go's
// build cache does not grow with every run, its replace pointed at this
// repository.
func (s *SkillCommandsTestSuite) BeforeEach(t *gotest.T) {
	root := repoRoot(t)
	s.dir = filepath.Join(os.TempDir(), fmt.Sprintf("gotest-skillcommands-%08x", crc32.ChecksumIEEE([]byte(root))), filepath.Base(t.Name()))
	gotest.NoError(t, os.RemoveAll(s.dir))
	src := filepath.Join(root, "tests", "skillevals", "writing-gotest-tests", "consumer-fixture")
	gotest.NoError(t, copyTree(src, s.dir))
	goMod := filepath.Join(s.dir, "go.mod")
	data, err := os.ReadFile(goMod) //nolint:gosec // G304: the staged fixture
	gotest.NoError(t, err)
	gotest.Contains(t, string(data), "=> ../../../../")
	data = bytes.Replace(data, []byte("=> ../../../../"), []byte("=> "+root), 1)
	gotest.NoError(t, os.WriteFile(goMod, data, 0o600))
}

func (s *SkillCommandsTestSuite) AfterEach(t *gotest.T) {
	gotest.NoError(t, os.RemoveAll(s.dir))
}

// copyTree copies the regular files under src to dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(p) //nolint:gosec // G304: the repository's own fixture
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
}

// sh runs script under bash with pipefail in the staged fixture.
func (s *SkillCommandsTestSuite) sh(t *gotest.T, script string, env ...string) (stdout, stderr string, exit int) {
	cmd := exec.CommandContext(t.Context(), "bash", "-o", "pipefail", "-c", script)
	cmd.Dir = s.dir
	cmd.Env = append(testkit.ChildEnv(s.dir), append([]string{"GOTEST_CACHE_DIR=" + filepath.Join(s.dir, ".gotest-cache"), "GOFLAGS="}, env...)...)
	cmd.WaitDelay = 5 * time.Second
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		exit = exitErr.ExitCode()
	case err != nil:
		gotest.NoError(t, err, "bash did not run: %s", script)
	}
	return out.String(), errOut.String(), exit
}

// shBlocks returns the bodies of the sh fences of SKILL.md, in order, with
// the fence's indentation removed.
func shBlocks(t *gotest.T) []string {
	var lines []string
	for _, f := range skillFiles(t) {
		if f.rel == "SKILL.md" {
			lines = f.lines
		}
	}
	var blocks []string
	var body []string
	indent, open := "", false
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		switch {
		case !open && trimmed == "```sh":
			indent, open, body = line[:len(line)-len(trimmed)], true, nil
		case open && trimmed == "```":
			blocks = append(blocks, strings.Join(body, "\n"))
			open = false
		case open:
			body = append(body, strings.TrimPrefix(line, indent))
		}
	}
	gotest.Len(t, blocks, 4, "SKILL.md's sh blocks: the version check, the two runners, the lint pair and the case capture")
	return blocks
}

// shown formats a command's output for a failure message.
func shown(line, stdout, stderr string, exit int) string {
	return fmt.Sprintf("command: %s\nexit: %d\nstdout:\n%s\nstderr:\n%s", line, exit, stdout, stderr)
}

func (s *SkillCommandsTestSuite) TestTheVersionCheckRuns(t *gotest.T) {
	lines := strings.Split(shBlocks(t)[0], "\n")
	gotest.Len(t, lines, 3)
	t.When("SKILL.md's version check block runs in a consumer module", func(w *gotest.T) {
		for i, line := range lines {
			stdout, stderr, exit := s.sh(w, line, "GOPROXY=off")
			w.It(fmt.Sprintf("line %d runs, or only its lookup is refused offline", i+1), func(it *gotest.T) {
				if exit != 0 {
					gotest.Contains(it, stderr, "GOPROXY=off", "%s", shown(line, stdout, stderr, exit))
				}
			})
			if i == 1 {
				w.It("line 2 prints the module's go version", func(it *gotest.T) {
					gotest.Regexp(it, `^\d+\.\d+(\.\d+)?\n$`, stdout, "%s", shown(line, stdout, stderr, exit))
				})
			}
		}
	})
}

func (s *SkillCommandsTestSuite) TestBothRunnersRun(t *gotest.T) {
	lines := strings.Split(shBlocks(t)[1], "\n")
	gotest.Len(t, lines, 2)
	t.When("SKILL.md's two runners block runs in a consumer module", func(w *gotest.T) {
		for i, line := range lines {
			stdout, stderr, exit := s.sh(w, line)
			w.It(fmt.Sprintf("line %d exits 0", i+1), func(it *gotest.T) {
				gotest.Zero(it, exit, "%s", shown(line, stdout, stderr, exit))
			})
			if i == 0 {
				w.It("the suite runner runs the suite package", func(it *gotest.T) {
					gotest.Regexp(it, `(?m)^ok\s+example\.com/shopd/store\b`, stdout, "%s", shown(line, stdout, stderr, exit))
				})
			}
		}
	})
}

func (s *SkillCommandsTestSuite) TestTheLintPairRuns(t *gotest.T) {
	lines := strings.Split(shBlocks(t)[2], "\n")
	gotest.Len(t, lines, 2)
	t.When("SKILL.md's lint pair block runs in a consumer module", func(w *gotest.T) {
		for i, line := range lines {
			stdout, stderr, exit := s.sh(w, line)
			w.It(fmt.Sprintf("line %d exits 0 or with findings", i+1), func(it *gotest.T) {
				gotest.Contains(it, []int{0, 3}, exit, "%s", shown(line, stdout, stderr, exit))
				gotest.NotContains(it, stderr, "unknown flag", "%s", shown(line, stdout, stderr, exit))
			})
		}
	})
}

func (s *SkillCommandsTestSuite) TestTheCaseCaptureRuns(t *gotest.T) {
	script := shBlocks(t)[3]
	stdout, stderr, exit := s.sh(t, script)
	cases, err := os.ReadFile(filepath.Join(s.dir, "cases-before.txt"))
	t.When("SKILL.md's case capture block runs in a consumer module", func(w *gotest.T) {
		w.It("exits 0", func(it *gotest.T) {
			gotest.Zero(it, exit, "%s", shown(script, stdout, stderr, exit))
		})
		w.It("lists the cases of both runners by package and test", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Contains(it, string(cases), `"Package":"example.com/shopd/store","Test":"TestStoreTestSuite/TestReserve1"`, "%s", string(cases))
			gotest.Contains(it, string(cases), `"Package":"example.com/shopd/pricing","Test":"TestTotal"`, "%s", string(cases))
		})
	})
}
