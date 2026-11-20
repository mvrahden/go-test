package gotestgen

import (
	"bytes"
	"fmt"
	"go/format"
	"go/types"
	"maps"
	"slices"
	"text/template"

	"github.com/mvrahden/go-test/internal/about"
)

// nodeID is the fixture's DAG node name in a generated harness.
func (sf *SharedFixtureInfo) nodeID() string {
	switch {
	case sf.IDPrefix != "":
		return sf.IDPrefix + "_" + sf.Identifier
	case sf.PkgName != "":
		return sf.PkgName + "_" + sf.Identifier
	}
	return sf.Identifier
}

var sharedFixtureTpl = template.Must(template.New("sharedfixture").ParseFS(templates, "static/sharedfixture.go.tpl"))

// SharedFixtureInfo describes a shared fixture to be run in a setup subprocess.
type SharedFixtureInfo struct {
	Identifier       string       // type name e.g. "PostgresFixture"
	PkgPath          string       // import path e.g. "github.com/example/project/tests/fixtures"
	PkgName          string       // Go package name e.g. "fixtures" (from types.Package.Name())
	QualifiedType    string       // Go type expression e.g. "fixtures.PostgresFixture" (handles generics)
	IDPrefix         string       // prefix of its generated identifiers, empty if same package
	Named            *types.Named // the fixture type
	HasConfig        bool
	HasAfterAll      bool
	HasHydrate       bool
	HasDehydrate     bool
	HydrateAt        string            // file:line of Hydrate, relative to the suite's directory; empty without one
	DehydrateAt      string            // the same for Dehydrate
	TransferFields   []string          // exported fields that are serialized (all exported minus local)
	LocalFields      []string          // exported fields assigned in Hydrate
	Dependencies     []string          // state keys of shared fixtures this one depends on
	DependencyFields map[string]string // dep state key → field name in this struct
	// Deferred is runner scheduling, not discovery output: a deferred fixture
	// is compiled into the setup program but not started up-front — it starts
	// when a StartKeys command names it (the bulk→tail barrier).
	Deferred bool
}

type sharedSetupData struct {
	RepoInfo string
	// GotestRuntimeImportPath runs the program: the generated source only
	// describes the fixtures to gotestruntime.RunSharedFixtures.
	GotestRuntimeImportPath string
	Imports                 []sharedSetupImport
	Fixtures                []sharedSetupFixture
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
	VarName           string
	QualifiedType     string
	Identifier        string
	StateKey          string
	HasConfig         bool
	HasAfterAll       bool
	TransferFields    []string
	DependsOnKeys     []string           // state keys of the program's fixtures this one binds, in order
	ParentAssignments []parentAssignment // parent var → field name assignments
	Deferred          bool               // excluded from the up-front phase; starts on StartKeys
}

// GenerateSharedSetup generates the shared fixture setup program: a main
// package that describes the fixtures to gotestruntime.RunSharedFixtures,
// which starts them, reports their state and serves the runner's commands.
func GenerateSharedSetup(fixtures []SharedFixtureInfo) ([]byte, error) {
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("no shared fixtures to generate setup for")
	}

	var imports []sharedSetupImport
	pkgAlias := map[string]string{}
	for i := range fixtures {
		if _, ok := pkgAlias[fixtures[i].PkgPath]; !ok {
			alias := fmt.Sprintf("sfpkg%d", len(imports))
			pkgAlias[fixtures[i].PkgPath] = alias
			imports = append(imports, sharedSetupImport{Alias: alias, Path: fixtures[i].PkgPath})
		}
	}

	// State key → var name, for dependency resolution.
	stateKeyToVar := make(map[string]string, len(fixtures))
	for i := range fixtures {
		stateKeyToVar[fixtures[i].PkgPath+"."+fixtures[i].Identifier] = fmt.Sprintf("sf%d", i)
	}

	var fixtureVMs []sharedSetupFixture
	for i := range fixtures {
		sf := &fixtures[i]
		varName := fmt.Sprintf("sf%d", i)
		alias := pkgAlias[sf.PkgPath]

		var dependsOnKeys []string
		for _, depKey := range sf.Dependencies {
			if _, ok := stateKeyToVar[depKey]; ok {
				dependsOnKeys = append(dependsOnKeys, depKey)
			}
		}

		// Ranged in sorted key order: generated code is a pure function of its
		// input, and Go map order would shuffle the assignment statements from
		// run to run — snapshot flakes and build-cache misses for free.
		var parentAssigns []parentAssignment
		for _, depKey := range slices.Sorted(maps.Keys(sf.DependencyFields)) {
			if v, ok := stateKeyToVar[depKey]; ok {
				parentAssigns = append(parentAssigns, parentAssignment{
					ParentVar: v,
					FieldName: sf.DependencyFields[depKey],
				})
			}
		}

		fixtureVMs = append(fixtureVMs, sharedSetupFixture{
			VarName:           varName,
			QualifiedType:     alias + "." + sf.Identifier,
			Identifier:        sf.Identifier,
			StateKey:          sf.PkgPath + "." + sf.Identifier,
			HasConfig:         sf.HasConfig,
			HasAfterAll:       sf.HasAfterAll,
			TransferFields:    sf.TransferFields,
			DependsOnKeys:     dependsOnKeys,
			ParentAssignments: parentAssigns,
			Deferred:          sf.Deferred,
		})
	}

	data := sharedSetupData{
		RepoInfo:                about.ShortInfo(),
		GotestRuntimeImportPath: about.Repo + "/pkg/gotestruntime",
		Imports:                 imports,
		Fixtures:                fixtureVMs,
	}

	var buf bytes.Buffer
	if err := sharedFixtureTpl.ExecuteTemplate(&buf, "sharedfixture.go.tpl", data); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	return format.Source(buf.Bytes())
}
