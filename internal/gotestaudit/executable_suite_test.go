package gotestaudit_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditExecutableTestSuite: a changed file counts where a changed line lies
// in a function's body.
type AuditExecutableTestSuite struct{}

// executableCtx is a test's own directory.
type executableCtx struct{ dir string }

func (s *AuditExecutableTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *AuditExecutableTestSuite) BeforeEach(t *gotest.T) *executableCtx {
	dir, err := os.MkdirTemp("", "gotest-audit-executable-")
	gotest.NoError(t, err)
	return &executableCtx{dir: dir}
}

func (s *AuditExecutableTestSuite) AfterEach(t *gotest.T, c *executableCtx) {
	if c.dir != "" {
		gotest.NoError(t, os.RemoveAll(c.dir))
	}
}

const executableSrc = `package p

// T is a type.
type T struct {
	N int
}

func One() int { return 1 }

func Two(
	n int,
) int {
	if n > 0 {
		return 2
	}
	return 0
}

func Empty() {}

var Lit = func() int {
	return 3
}
`

func (s *AuditExecutableTestSuite) TestLinesOfFunctions(t *gotest.T, c *executableCtx) {
	file := filepath.Join(c.dir, "p.go")
	gotest.NoError(t, os.WriteFile(file, []byte(executableSrc), 0o600))
	for row, r := range gotest.Each(t, []struct {
		Desc  string
		lines []int
		kept  int
	}{
		{Desc: "a type's lines", lines: []int{4, 5, 6}, kept: 0},
		{Desc: "a function on one line", lines: []int{8}, kept: 1},
		{Desc: "a function's signature", lines: []int{10, 11, 12}, kept: 0},
		{Desc: "a function's first statement", lines: []int{13}, kept: 1},
		{Desc: "a function's last statement", lines: []int{16}, kept: 1},
		{Desc: "a function's closing brace", lines: []int{17}, kept: 0},
		{Desc: "a function with no statement", lines: []int{19}, kept: 0},
		{Desc: "a literal's opening line", lines: []int{21}, kept: 0},
		{Desc: "a literal's statement", lines: []int{22}, kept: 1},
		{Desc: "the package clause and a comment", lines: []int{1, 3}, kept: 0},
	}) {
		got := gotestaudit.Executable([]gotestaudit.ProdFile{{File: "p.go", Package: "m/p", Lines: r.lines, Abs: file}})
		gotest.Len(row, got, r.kept)
	}
	t.When("the file is not there", func(w *gotest.T) {
		w.It("keeps it", func(it *gotest.T) {
			got := gotestaudit.Executable([]gotestaudit.ProdFile{{File: "q.go", Package: "m/p", Lines: []int{1}, Abs: filepath.Join(c.dir, "q.go")}})
			gotest.Len(it, got, 1)
		})
	})
}
