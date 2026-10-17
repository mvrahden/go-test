package main_test

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// SkillCLITestSuite drives "gotest skill install" through the built binary:
// it writes the repository's skill, replaces an older copy of it, and never
// touches a directory holding anything else.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type SkillCLITestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *SkillCLITestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type skillCtx struct {
	dir    string // project directory the command runs in
	target string // where the skill lands under --dir=skills
}

func (s *SkillCLITestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

func (s *SkillCLITestSuite) BeforeEach(t *gotest.T) *skillCtx {
	dir := t.TempDir()
	return &skillCtx{dir: dir, target: filepath.Join(dir, "skills", "writing-gotest-tests")}
}

func (s *SkillCLITestSuite) install(t *gotest.T, ctx *skillCtx, args ...string) (string, int) {
	return runGotestIn(t, s.cli.binary, ctx.dir, nil, append([]string{"skill", "install"}, args...)...)
}

func (s *SkillCLITestSuite) source(t *gotest.T, rel string) string {
	data, err := os.ReadFile(filepath.Join(s.cli.repoRoot, "skills", "writing-gotest-tests", rel))
	gotest.NoError(t, err)
	return string(data)
}

func readText(t *gotest.T, path string) string {
	data, err := os.ReadFile(path)
	gotest.NoError(t, err)
	return string(data)
}

func (s *SkillCLITestSuite) TestInstallDefault(t *gotest.T, ctx *skillCtx) {
	out, code := s.install(t, ctx)
	t.It("writes the skill into .agents/skills and prints where", func(it *gotest.T) {
		gotest.Equal(it, 0, code, out)
		gotest.Equal(it, filepath.Join(".agents", "skills", "writing-gotest-tests")+"\n", out)
		gotest.Equal(it, s.source(it, "SKILL.md"), readText(it, filepath.Join(ctx.dir, ".agents", "skills", "writing-gotest-tests", "SKILL.md")))
	})
}

func (s *SkillCLITestSuite) TestInstallDir(t *gotest.T, ctx *skillCtx) {
	out, code := s.install(t, ctx, "--dir=skills")
	t.It("writes SKILL.md and every reference file as the repository has them", func(it *gotest.T) {
		gotest.Equal(it, 0, code, out)
		gotest.Equal(it, s.source(it, "SKILL.md"), readText(it, filepath.Join(ctx.target, "SKILL.md")))
		refs, err := os.ReadDir(filepath.Join(s.cli.repoRoot, "skills", "writing-gotest-tests", "reference"))
		gotest.NoError(it, err)
		gotest.NotEmpty(it, refs)
		for _, ref := range refs {
			rel := filepath.Join("reference", ref.Name())
			gotest.Equal(it, s.source(it, rel), readText(it, filepath.Join(ctx.target, rel)), rel)
		}
	})
}

func (s *SkillCLITestSuite) TestReplacesOlderCopy(t *gotest.T, ctx *skillCtx) {
	gotest.NoError(t, os.MkdirAll(filepath.Join(ctx.target, "reference"), 0o755))
	gotest.NoError(t, os.WriteFile(filepath.Join(ctx.target, "SKILL.md"), []byte("---\nname: writing-gotest-tests\n---\nold\n"), 0o600))
	stale := filepath.Join(ctx.target, "reference", "removed.md")
	gotest.NoError(t, os.WriteFile(stale, []byte("gone in this version"), 0o600))
	out, code := s.install(t, ctx, "--dir=skills")
	t.It("replaces it, dropping files this version no longer ships", func(it *gotest.T) {
		gotest.Equal(it, 0, code, out)
		gotest.Equal(it, s.source(it, "SKILL.md"), readText(it, filepath.Join(ctx.target, "SKILL.md")))
		_, err := os.Stat(stale)
		gotest.ErrorIs(it, err, os.ErrNotExist)
	})
}

func (s *SkillCLITestSuite) TestReplaceLeavesNothingBehind(t *gotest.T, ctx *skillCtx) {
	_, code := s.install(t, ctx, "--dir=skills")
	gotest.Equal(t, 0, code)
	out, code := s.install(t, ctx, "--dir=skills")
	t.It("leaves only the skill, with no staged or replaced copy beside it", func(it *gotest.T) {
		gotest.Equal(it, 0, code, out)
		for _, dir := range []string{ctx.dir, filepath.Join(ctx.dir, "skills")} {
			entries, err := os.ReadDir(dir)
			gotest.NoError(it, err)
			for _, e := range entries {
				gotest.NotContains(it, e.Name(), "-new-", dir)
			}
		}
	})
}

func (s *SkillCLITestSuite) TestFailedInstallKeepsOldCopy(t *gotest.T, ctx *skillCtx) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skipf("needs a directory the user cannot write")
	}
	old := "---\nname: writing-gotest-tests\n---\nold\n"
	gotest.NoError(t, os.MkdirAll(ctx.target, 0o755))
	gotest.NoError(t, os.WriteFile(filepath.Join(ctx.target, "SKILL.md"), []byte(old), 0o600))
	// The new copy is staged beside the skills directory; with no room there
	// the install fails before it touches the old copy.
	gotest.NoError(t, os.Chmod(ctx.dir, 0o555))
	out, code := s.install(t, ctx, "--dir=skills")
	gotest.NoError(t, os.Chmod(ctx.dir, 0o755))
	t.It("exits 2 and leaves the installed copy as it was", func(it *gotest.T) {
		gotest.Equal(it, 2, code, out)
		gotest.Equal(it, old, readText(it, filepath.Join(ctx.target, "SKILL.md")))
	})
}

func (s *SkillCLITestSuite) TestRefusesForeignSkill(t *gotest.T, ctx *skillCtx) {
	gotest.NoError(t, os.MkdirAll(ctx.target, 0o755))
	foreign := "---\nname: someone-elses-skill\n---\nmine\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(ctx.target, "SKILL.md"), []byte(foreign), 0o600))
	out, code := s.install(t, ctx, "--dir=skills")
	t.It("exits 2 naming it and leaves it untouched", func(it *gotest.T) {
		gotest.Equal(it, 2, code, out)
		gotest.Contains(it, out, `holds skill "someone-elses-skill"`)
		gotest.Equal(it, foreign, readText(it, filepath.Join(ctx.target, "SKILL.md")))
	})
}

func (s *SkillCLITestSuite) TestRefusesDirWithoutSkill(t *gotest.T, ctx *skillCtx) {
	gotest.NoError(t, os.MkdirAll(ctx.target, 0o755))
	notes := filepath.Join(ctx.target, "notes.txt")
	gotest.NoError(t, os.WriteFile(notes, []byte("keep me"), 0o600))
	out, code := s.install(t, ctx, "--dir=skills")
	t.It("exits 2 and keeps its files", func(it *gotest.T) {
		gotest.Equal(it, 2, code, out)
		gotest.Contains(it, out, "holds no SKILL.md")
		gotest.Equal(it, "keep me", readText(it, notes))
	})
}

func (s *SkillCLITestSuite) TestUsage(t *gotest.T, ctx *skillCtx) {
	t.It("prints usage and exits 2 without a command", func(it *gotest.T) {
		out, code := runGotestIn(it, s.cli.binary, ctx.dir, nil, "skill")
		gotest.Equal(it, 2, code)
		gotest.Contains(it, out, "usage: gotest skill install")
	})
	t.It("rejects an unknown command", func(it *gotest.T) {
		out, code := runGotestIn(it, s.cli.binary, ctx.dir, nil, "skill", "uninstall")
		gotest.Equal(it, 2, code)
		gotest.Contains(it, out, "unknown skill command: uninstall")
	})
	t.It("rejects an empty --dir", func(it *gotest.T) {
		out, code := s.install(it, ctx, "--dir=")
		gotest.Equal(it, 2, code)
		gotest.Contains(it, out, "--dir needs a directory")
	})
	t.It("rejects a positional argument", func(it *gotest.T) {
		out, code := s.install(it, ctx, "somewhere")
		gotest.Equal(it, 2, code)
		gotest.Contains(it, out, "unexpected argument somewhere")
	})
}
