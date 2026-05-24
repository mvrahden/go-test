package gotestgen_test

import (
	"go/types"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// LinknameCompatTestSuite tests go:linkname compatibility detection
// for struct layout and coverage report signatures.
type LinknameCompatTestSuite struct{}

func (s *LinknameCompatTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func loadTestingScope(t testing.TB) *types.Scope {
	t.Helper()
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedSyntax}
	pkgs, err := packages.Load(cfg, "testing")
	gotest.NoError(t, err)
	gotest.True(t, len(pkgs) > 0)
	return pkgs[0].Types.Scope()
}

func (s *LinknameCompatTestSuite) TestCoverStructLayout(t *gotest.T) {
	t.When("testing package cover struct", func(w *gotest.T) {
		w.It("has the expected field layout", func(it *gotest.T) {
			scope := loadTestingScope(it.T())

			// Go 1.25+ uses "cover", Go 1.24 uses "cover2".
			varName := "cover"
			if scope.Lookup("cover2") != nil {
				varName = "cover2"
			}

			obj := scope.Lookup(varName)
			gotest.True(it, obj != nil, "testing.%s variable not found", varName)

			st, ok := obj.Type().Underlying().(*types.Struct)
			gotest.True(it, ok, "testing.%s is %s, expected struct", varName, obj.Type())

			type field struct {
				name string
				typ  string
			}
			want := []field{
				{"mode", "string"},
				{"tearDown", "func(coverprofile string, gocoverdir string) (string, error)"},
				{"snapshotcov", "func() float64"},
			}

			gotest.Equal(it, len(want), st.NumFields(), "testing.%s has %d fields, expected %d", varName, st.NumFields(), len(want))
			for i, w := range want {
				f := st.Field(i)
				gotest.Equal(it, w.name, f.Name(), "field %d: name mismatch", i)
				gotest.Equal(it, w.typ, f.Type().String(), "field %d (%s): type mismatch", i, w.name)
			}
		})
	})
}

func (s *LinknameCompatTestSuite) TestCoverReportSignature(t *gotest.T) {
	t.When("testing.coverReport function", func(w *gotest.T) {
		w.It("exists with zero params and zero results", func(it *gotest.T) {
			scope := loadTestingScope(it.T())

			obj := scope.Lookup("coverReport")
			gotest.True(it, obj != nil, "testing.coverReport function not found — may have been renamed or removed")

			sig, ok := obj.Type().(*types.Signature)
			gotest.True(it, ok, "testing.coverReport is %s, expected function", obj.Type())
			gotest.Equal(it, 0, sig.Params().Len(), "testing.coverReport has %d params, expected 0", sig.Params().Len())
			gotest.Equal(it, 0, sig.Results().Len(), "testing.coverReport has %d results, expected 0", sig.Results().Len())
		})
	})
}
