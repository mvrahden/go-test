package gotestgen //nolint:stdlib-test

import (
	"go/types"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

func loadTestingScope(t *testing.T) *types.Scope {
	t.Helper()
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedSyntax}
	pkgs, err := packages.Load(cfg, "testing")
	gotest.NoError(t, err)
	gotest.True(t, len(pkgs) > 0)
	return pkgs[0].Types.Scope()
}

func TestLinknameCompat_CoverStructLayout(t *testing.T) {
	scope := loadTestingScope(t)

	// Go 1.25+ uses "cover", Go 1.24 uses "cover2".
	varName := "cover"
	if scope.Lookup("cover2") != nil {
		varName = "cover2"
	}

	obj := scope.Lookup(varName)
	if obj == nil {
		t.Fatalf("testing.%s variable not found", varName)
	}
	st, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("testing.%s is %s, expected struct", varName, obj.Type())
	}

	type field struct {
		name string
		typ  string
	}
	want := []field{
		{"mode", "string"},
		{"tearDown", "func(coverprofile string, gocoverdir string) (string, error)"},
		{"snapshotcov", "func() float64"},
	}

	if st.NumFields() != len(want) {
		t.Fatalf("testing.%s has %d fields, expected %d", varName, st.NumFields(), len(want))
	}
	for i, w := range want {
		f := st.Field(i)
		if f.Name() != w.name {
			t.Errorf("field %d: name = %q, want %q", i, f.Name(), w.name)
		}
		if f.Type().String() != w.typ {
			t.Errorf("field %d (%s): type = %q, want %q", i, w.name, f.Type().String(), w.typ)
		}
	}
}

func TestLinknameCompat_CoverReportSignature(t *testing.T) {
	scope := loadTestingScope(t)

	obj := scope.Lookup("coverReport")
	if obj == nil {
		t.Fatal("testing.coverReport function not found — may have been renamed or removed")
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		t.Fatalf("testing.coverReport is %s, expected function", obj.Type())
	}
	if sig.Params().Len() != 0 {
		t.Errorf("testing.coverReport has %d params, expected 0", sig.Params().Len())
	}
	if sig.Results().Len() != 0 {
		t.Errorf("testing.coverReport has %d results, expected 0", sig.Results().Len())
	}
}
