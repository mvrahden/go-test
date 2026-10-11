package scaffold_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/scaffold"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ContractScaffoldTestSuite runs the scaffolded contract suite the way its
// closing comment says to use it: through a type alias, which cannot set fields.
type ContractScaffoldTestSuite struct{}

// SuiteConfig: each case builds a scratch module, which the 30-second default
// does not cover on a slow machine.
func (s *ContractScaffoldTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

// contractDriver instantiates the scaffold via an alias and runs its BeforeEach.
const contractDriver = `package sampletype

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type impl struct{}

func (*impl) Validate(string) error { return nil }
func (*impl) IsValid(string) bool   { return true }

type ImplValidatorTestSuite = ValidatorContractTestSuite[*impl]

func TestBeforeEach(t *testing.T) {
	s := &ImplValidatorTestSuite{}
	s.BeforeEach(gotest.NewT(t))
	gotest.NotZero(t, s.sut)
}
`

// runContract runs `go test` on the scaffold plus contractDriver in a module
// that replaces github.com/mvrahden/go-test with this worktree.
func runContract(t *gotest.T, src string) (stdout, stderr string, err error) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	typesSrc, err := os.ReadFile("testdata/sampletype/types.go")
	gotest.NoError(t, err)

	dir := t.TempDir()
	mod := "module contractcheck\n\ngo 1.24\n\nrequire github.com/mvrahden/go-test v0.0.0\n\nreplace github.com/mvrahden/go-test => " + repoRoot + "\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "types.go"), typesSrc, 0600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "validator_suite_test.go"), []byte(src), 0600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "driver_test.go"), []byte(contractDriver), 0600))

	// -mod=mod: the module has no go.sum, and needs none beyond the replace.
	cmd := exec.CommandContext(t.Context(), "go", "test", "-mod=mod", "-count=1", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func (s *ContractScaffoldTestSuite) TestRunsUnderAnAlias(t *gotest.T) {
	info, err := scaffold.IntrospectType("./testdata/sampletype", "Validator")
	gotest.NoError(t, err)
	out, err := scaffold.GenerateContractScaffold(info)
	gotest.NoError(t, err)
	src := string(out)

	t.When("no case constructs the implementation", func(w *gotest.T) {
		stdout, stderr, err := runContract(w, src)

		w.It("fails naming the missing case, not with a nil panic", func(it *gotest.T) {
			gotest.Error(it, err)
			gotest.Contains(it, stdout, "no constructor for *sampletype.impl: add a case to BeforeEach")
			gotest.NotContains(it, stdout, "nil pointer dereference")
			gotest.NotContains(it, stderr, "nil pointer dereference")
		})
	})

	t.When("the commented example case is filled in", func(w *gotest.T) {
		const example = "\t// case *MyImpl:\n\t// \ts.sut = any(NewMyImpl()).(T)\n"
		gotest.Contains(w, src, example)
		filled := strings.Replace(src, example, "\tcase *impl:\n\t\ts.sut = any(&impl{}).(T)\n", 1)

		stdout, stderr, err := runContract(w, filled)

		w.It("constructs it", func(it *gotest.T) {
			gotest.NoError(it, err, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
		})
	})
}
