package gotestgen

import (
	"bytes"
	"fmt"
	"go/format"
	"text/template"

	"github.com/mvrahden/go-test/about"
)

var sharedFixtureTpl = template.Must(template.New("sharedfixture").ParseFS(templates, "static/sharedfixture.go.tpl"))

// SharedFixtureInfo describes a shared fixture to be run in a setup subprocess.
type SharedFixtureInfo struct {
	Identifier     string // type name e.g. "PostgresFixture"
	PkgPath        string // import path e.g. "github.com/example/project/tests/fixtures"
	HasConfig      bool
	HasHydrate     bool
	HasDehydrate   bool
	TransferFields []string // exported fields that are serialized (all exported minus local)
	LocalFields    []string // exported fields assigned in Hydrate
}

type sharedSetupData struct {
	RepoInfo     string
	Imports      []sharedSetupImport
	Fixtures     []sharedSetupFixture
	TeardownVars []string
}

type sharedSetupImport struct {
	Alias string
	Path  string
}

type sharedSetupFixture struct {
	VarName               string
	QualifiedType         string
	Identifier            string
	StateKey              string
	TransferFields        []string
	BeforeAllRollbackVars []string
	MarshalRollbackVars   []string
}

// GenerateSharedSetup generates a standalone Go main package source that
// starts shared fixtures, serializes transferable fields as JSON to stdout,
// then waits for SIGTERM/SIGINT before tearing down.
func GenerateSharedSetup(fixtures []SharedFixtureInfo) ([]byte, error) {
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("no shared fixtures to generate setup for")
	}

	var imports []sharedSetupImport
	pkgAlias := map[string]string{}
	for _, sf := range fixtures {
		if _, ok := pkgAlias[sf.PkgPath]; !ok {
			alias := fmt.Sprintf("sfpkg%d", len(imports))
			pkgAlias[sf.PkgPath] = alias
			imports = append(imports, sharedSetupImport{Alias: alias, Path: sf.PkgPath})
		}
	}

	var fixtureVMs []sharedSetupFixture
	for i, sf := range fixtures {
		varName := fmt.Sprintf("sf%d", i)
		alias := pkgAlias[sf.PkgPath]

		beforeAllRollback := make([]string, 0, i)
		for j := i - 1; j >= 0; j-- {
			beforeAllRollback = append(beforeAllRollback, fmt.Sprintf("sf%d", j))
		}

		marshalRollback := make([]string, 0, i+1)
		for j := i; j >= 0; j-- {
			marshalRollback = append(marshalRollback, fmt.Sprintf("sf%d", j))
		}

		fixtureVMs = append(fixtureVMs, sharedSetupFixture{
			VarName:               varName,
			QualifiedType:         alias + "." + sf.Identifier,
			Identifier:            sf.Identifier,
			StateKey:              sf.PkgPath + "." + sf.Identifier,
			TransferFields:        sf.TransferFields,
			BeforeAllRollbackVars: beforeAllRollback,
			MarshalRollbackVars:   marshalRollback,
		})
	}

	teardownVars := make([]string, 0, len(fixtures))
	for i := len(fixtures) - 1; i >= 0; i-- {
		teardownVars = append(teardownVars, fmt.Sprintf("sf%d", i))
	}

	data := sharedSetupData{
		RepoInfo:     about.ShortInfo(),
		Imports:      imports,
		Fixtures:     fixtureVMs,
		TeardownVars: teardownVars,
	}

	var buf bytes.Buffer
	if err := sharedFixtureTpl.ExecuteTemplate(&buf, "sharedfixture.go.tpl", data); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	return format.Source(buf.Bytes())
}
