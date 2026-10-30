package genstub_test

import (
	"go/token"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/genstub"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// GenStubTestSuite covers the overlay that stands a package clause in for
// every generated file on disk while packages load.
// Sequential: Setenv.
type GenStubTestSuite struct{}

// cleanupCtx holds what a test's overlays leave, for AfterEach to remove.
type cleanupCtx struct{ cleanups []func() }

func (s *GenStubTestSuite) BeforeEach(_ *gotest.T) *cleanupCtx { return &cleanupCtx{} }

func (s *GenStubTestSuite) AfterEach(_ *gotest.T, ctx *cleanupCtx) {
	for _, c := range ctx.cleanups {
		c()
	}
}

// staleModule writes a module whose package p holds both generated files,
// each naming a suite that no longer exists.
func staleModule(t *gotest.T) string {
	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module stubprobe\n\ngo 1.25\n"), 0o600))
	pkg := filepath.Join(dir, "p")
	gotest.NoError(t, os.MkdirAll(pkg, 0o755))
	for name, src := range map[string]string{
		"p.go":        "package p\n",
		"p_test.go":   "package p\n\nimport \"testing\"\n\nfunc TestP(t *testing.T) {}\n",
		about.PSuite:  "package p\n\nvar _ = &OldTestSuite{}\n",
		about.PXSuite: "package p_test\n\nvar _ = Old\n",
	} {
		gotest.NoError(t, os.WriteFile(filepath.Join(pkg, name), []byte(src), 0o600))
	}
	return dir
}

func (s *GenStubTestSuite) TestStubs(t *gotest.T, _ *cleanupCtx) {
	t.When("the patterns match a package holding generated files", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := staleModule(w)
		stubs := genstub.Stubs(dir, []string{"./..."}, nil)

		w.It("stubs each with its package clause", func(it *gotest.T) {
			gotest.Equal(it, map[string][]byte{
				filepath.Join(dir, "p", about.PSuite):  []byte("package p\n"),
				filepath.Join(dir, "p", about.PXSuite): []byte("package p_test\n"),
			}, stubs)
		})
	})
}

func (s *GenStubTestSuite) TestLoadFlags(t *gotest.T, ctx *cleanupCtx) {
	t.When("a package holds stale generated files", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := staleModule(w)
		flags, cleanup, err := genstub.LoadFlags(dir, []string{"./..."}, []string{"-tags=x"})
		gotest.NoError(w, err)
		ctx.cleanups = append(ctx.cleanups, cleanup)

		w.It("keeps the given flags and adds the overlay", func(it *gotest.T) {
			gotest.Len(it, flags, 2)
			gotest.Equal(it, "-tags=x", flags[0])
			gotest.Regexp(it, "^-overlay=", flags[1])
		})
		w.It("lets the package load with types", func(it *gotest.T) {
			cfg := &packages.Config{
				Dir:        dir,
				Mode:       packages.LoadSyntax,
				Tests:      true,
				Fset:       token.NewFileSet(),
				BuildFlags: flags,
				ParseFile:  about.ParseStubbingGenerated,
			}
			pkgs, err := packages.Load(cfg, "./...")
			gotest.NoError(it, err)
			for _, p := range pkgs {
				gotest.Empty(it, p.Errors, p.ID)
			}
		})
	})
	t.When("no package holds a generated file", func(w *gotest.T) {
		w.Setenv("GOWORK", "off")
		dir := w.TempDir()
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module stubprobe\n\ngo 1.25\n"), 0o600))
		gotest.NoError(w, os.WriteFile(filepath.Join(dir, "p.go"), []byte("package stubprobe\n"), 0o600))
		flags, cleanup, err := genstub.LoadFlags(dir, []string{"./..."}, []string{"-tags=x"})
		gotest.NoError(w, err)
		ctx.cleanups = append(ctx.cleanups, cleanup)

		w.It("leaves the flags as they are", func(it *gotest.T) {
			gotest.Equal(it, []string{"-tags=x"}, flags)
		})
	})
}
