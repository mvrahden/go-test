package scaffold_test

import (
	"github.com/mvrahden/go-test/internal/scaffold"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScaffoldRefusalsTestSuite covers the targets introspection refuses because
// no scaffold of them would compile.
type ScaffoldRefusalsTestSuite struct{}

func (s *ScaffoldRefusalsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func introspectType(pkg, name string) func() error {
	return func() error {
		_, err := scaffold.IntrospectType(pkg, name)
		return err
	}
}

func introspectFuzz(pkg, name string) func() error {
	return func() error {
		_, err := scaffold.IntrospectFuzzTarget(pkg, name)
		return err
	}
}

func introspectFile(dir, name string) func() error {
	return func() error {
		_, err := scaffold.IntrospectFile(dir, name)
		return err
	}
}

func (s *ScaffoldRefusalsTestSuite) TestGenericTargets(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc       string
		introspect func() error
		want       string
	}{
		{"a generic struct", introspectType("./testdata/generic", "Index"), "Index is generic (type Index[T any])"},
		{"a generic interface", introspectType("./testdata/generic", "Store"), "Store is generic (type Store[K comparable])"},
		{"a generic function of one parameter", introspectFuzz("./testdata/generic", "Ident"), "Ident is generic (func Ident[T any])"},
		{"a generic function of two parameters", introspectFuzz("./testdata/generic", "Map"), "Map is generic (func Map[T any, U any])"},
		{"a method of a generic type", introspectType("./testdata/generic.Index", "Len"), "Index.Len is a method of a generic type: Index is generic (type Index[T any])"},
		{"a method of a generic type, fuzzed", introspectFuzz("./testdata/generic.Index", "Add"), "Index.Add is a method of a generic type: Index is generic (type Index[T any])"},
	}) {
		err := tc.introspect()
		gotest.ErrorContains(sub, err, tc.want)
	}
}

func (s *ScaffoldRefusalsTestSuite) TestMethodTargets(t *gotest.T) {
	err := introspectType("./testdata/generic.Plain", "Do")()
	gotest.ErrorContains(t, err, "Plain.Do is a method; scaffold its type instead: ./testdata/generic.Plain")
}

func (s *ScaffoldRefusalsTestSuite) TestTakenSuiteNames(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc       string
		introspect func() error
		want       []string
	}{
		{"a type whose suite an external test file declares", introspectType("./testdata/taken", "Ledger"), []string{"LedgerTestSuite is already declared: type LedgerTestSuite in package taken_test at ", "books_test.go:3"}},
		{"an interface whose contract suite a test file declares", introspectType("./testdata/taken", "Journal"), []string{"JournalContractTestSuite is already declared: func JournalContractTestSuite in package taken at ", "audit_test.go:5"}},
		{"a file whose suite the package declares", introspectFile("./testdata/taken", "taken.go"), []string{"TakenTestSuite is already declared: const TakenTestSuite in package taken at ", "taken.go:4"}},
		{"a function whose fuzz suite a test file declares", introspectFuzz("./testdata/taken", "Settle"), []string{"SettleTestSuite is already declared: var SettleTestSuite in package taken at ", "audit_test.go:3"}},
	}) {
		err := tc.introspect()
		for _, want := range tc.want {
			gotest.ErrorContains(sub, err, want)
		}
	}
}
