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
	Identifier       string // type name e.g. "PostgresFixture"
	PkgPath          string // import path e.g. "github.com/example/project/tests/fixtures"
	PkgName          string // Go package name e.g. "fixtures" (from types.Package.Name())
	QualifiedType    string // Go type expression e.g. "fixtures.PostgresFixture" (handles generics)
	HasConfig        bool
	HasHydrate       bool
	HasDehydrate     bool
	TransferFields   []string          // exported fields that are serialized (all exported minus local)
	LocalFields      []string          // exported fields assigned in Hydrate
	Dependencies     []string          // state keys of shared fixtures this one depends on
	DependencyFields map[string]string // dep state key → field name in this struct
}

type sharedSetupData struct {
	RepoInfo         string
	GotestImportPath string
	Imports          []sharedSetupImport
	Fixtures         []sharedSetupFixture
	TeardownFixtures []sharedSetupFixture // fixtures in reverse dependency order for teardown
}

type sharedSetupImport struct {
	Alias string
	Path  string
}

type parentAssignment struct {
	ParentVar string // e.g. "sf0"
	FieldName string // e.g. "Postgres"
}

type sharedSetupFixture struct {
	Index             int
	VarName           string
	QualifiedType     string
	Identifier        string
	StateKey          string
	HasConfig         bool
	TransferFields    []string
	DependsOnVars     []string           // var names of fixtures this depends on (e.g. ["sf0"])
	DependsOnIndices  []int              // indices of dependency fixtures in the Fixtures slice
	ParentAssignments []parentAssignment // parent var → field name assignments
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

	// Build a map from state key → var name and state key → index for dependency resolution.
	stateKeyToVar := make(map[string]string, len(fixtures))
	stateKeyToIndex := make(map[string]int, len(fixtures))
	for i, sf := range fixtures {
		key := sf.PkgPath + "." + sf.Identifier
		stateKeyToVar[key] = fmt.Sprintf("sf%d", i)
		stateKeyToIndex[key] = i
	}

	var fixtureVMs []sharedSetupFixture
	for i, sf := range fixtures {
		varName := fmt.Sprintf("sf%d", i)
		alias := pkgAlias[sf.PkgPath]

		var dependsOnVars []string
		var dependsOnIndices []int
		for _, depKey := range sf.Dependencies {
			v, vOk := stateKeyToVar[depKey]
			idx, idxOk := stateKeyToIndex[depKey]
			if vOk && idxOk {
				dependsOnVars = append(dependsOnVars, v)
				dependsOnIndices = append(dependsOnIndices, idx)
			}
		}

		var parentAssigns []parentAssignment
		for depKey, fieldName := range sf.DependencyFields {
			if v, ok := stateKeyToVar[depKey]; ok {
				parentAssigns = append(parentAssigns, parentAssignment{
					ParentVar: v,
					FieldName: fieldName,
				})
			}
		}

		fixtureVMs = append(fixtureVMs, sharedSetupFixture{
			Index:             i,
			VarName:           varName,
			QualifiedType:     alias + "." + sf.Identifier,
			Identifier:        sf.Identifier,
			StateKey:          sf.PkgPath + "." + sf.Identifier,
			HasConfig:         sf.HasConfig,
			TransferFields:    sf.TransferFields,
			DependsOnVars:     dependsOnVars,
			DependsOnIndices:  dependsOnIndices,
			ParentAssignments: parentAssigns,
		})
	}

	teardownFixtures := make([]sharedSetupFixture, len(fixtureVMs))
	for i, f := range fixtureVMs {
		teardownFixtures[len(fixtureVMs)-1-i] = f
	}

	data := sharedSetupData{
		RepoInfo:         about.ShortInfo(),
		GotestImportPath: about.Repo + "/pkg/gotest",
		Imports:          imports,
		Fixtures:         fixtureVMs,
		TeardownFixtures: teardownFixtures,
	}

	var buf bytes.Buffer
	if err := sharedFixtureTpl.ExecuteTemplate(&buf, "sharedfixture.go.tpl", data); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	return format.Source(buf.Bytes())
}
