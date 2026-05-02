package gotestgen

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"golang.org/x/tools/go/packages"
)

// ResolvedFixture represents a fixture resolved from the type graph.
// It carries all data needed for rendering and setup subprocess generation.
type ResolvedFixture struct {
	Kind            gotestast.FixtureKind
	Identifier      string // unqualified type name, e.g. "InfraFixture"
	QualifiedType   string // "pkg.Name" for cross-package, "Name" for same
	ParentFieldName string // field name in this fixture's struct that holds the parent fixture pointer
	PkgPath         string // import path, empty if same package
	PkgName         string // package name for qualified refs, empty if same package

	Pkg   *packages.Package
	Named *types.Named
	Spec  *gotestast.FixtureSpec // non-nil only for locally collected fixtures

	HasConfig    bool
	BeforeAll    bool
	AfterAll     bool
	BeforeEach   bool
	AfterEach    bool
	HasHydrate   bool
	HasDehydrate bool

	TransferFields []string // shared fixtures only
	LocalFields    []string // shared fixtures only

	Parent         *ResolvedFixture
	Children       []*ResolvedFixture
	SharedFixtures []SharedFixtureRef
	ChildSuites    []*gotestast.TestSuiteSpec
}

// ResolveResult is the output of fixture resolution for a target package.
type ResolveResult struct {
	RootFixtures           []*ResolvedFixture
	RequiredSharedFixtures []SharedFixtureInfo // deduplicated, for setup subprocess
	FixtureBound           []*gotestast.TestSuiteSpec
	Standalone             []*gotestast.TestSuiteSpec
	SuiteSharedFixtures    map[string][]SharedFixtureRef // suite identifier → direct shared fixture refs
}

type resolver struct {
	targetPkg     *packages.Package
	localFixtures []*gotestast.FixtureSpec
	resolved      map[*types.Named]*ResolvedFixture
	resolving     map[*types.Named]bool // cycle detection
	sharedSeen    map[string]*SharedFixtureInfo // key: pkgPath.Name
	result        *ResolveResult
}

// Resolve performs demand-driven fixture resolution starting from targeted test
// suites. It walks the type graph recursively to discover all required fixtures
// (both package and shared), validates constraints, and builds the fixture tree.
func Resolve(targetPkg *packages.Package, suites []*gotestast.TestSuiteSpec, localFixtures []*gotestast.FixtureSpec) (*ResolveResult, error) {
	result := &ResolveResult{}
	r := &resolver{
		targetPkg:     targetPkg,
		localFixtures: localFixtures,
		resolved:      make(map[*types.Named]*ResolvedFixture),
		resolving:     make(map[*types.Named]bool),
		sharedSeen:    make(map[string]*SharedFixtureInfo),
		result:        result,
	}

	for _, suite := range suites {
		fixture, err := r.resolveFixtureForSuite(suite)
		if err != nil {
			return nil, err
		}
		if fixture != nil {
			fixture.ChildSuites = append(fixture.ChildSuites, suite)
			suite.SetFixture(r.findLocalSpec(fixture))
			result.FixtureBound = append(result.FixtureBound, suite)
		} else {
			result.Standalone = append(result.Standalone, suite)
		}
	}

	// Collect unique root fixtures
	seen := make(map[*types.Named]bool)
	for _, rf := range r.resolved {
		if rf.Kind != gotestast.PackageFixture {
			continue
		}
		root := rf
		for root.Parent != nil {
			root = root.Parent
		}
		if seen[root.Named] {
			continue
		}
		seen[root.Named] = true
		if hasChildSuitesRecursive(root) {
			result.RootFixtures = append(result.RootFixtures, root)
		}
	}

	if len(result.RootFixtures) > 1 {
		names := make([]string, len(result.RootFixtures))
		for i, rf := range result.RootFixtures {
			names[i] = rf.Identifier
		}
		return nil, fmt.Errorf("at most one root package fixture per package is allowed, found: %s", strings.Join(names, ", "))
	}

	// Collect deduplicated shared fixtures
	for _, sf := range r.sharedSeen {
		result.RequiredSharedFixtures = append(result.RequiredSharedFixtures, *sf)
	}

	return result, nil
}

func hasChildSuitesRecursive(rf *ResolvedFixture) bool {
	if len(rf.ChildSuites) > 0 {
		return true
	}
	for _, child := range rf.Children {
		if hasChildSuitesRecursive(child) {
			return true
		}
	}
	return false
}

type suiteFixtureMatch struct {
	resolved  *ResolvedFixture
	fieldName string
}

func (r *resolver) resolveFixtureForSuite(suite *gotestast.TestSuiteSpec) (*ResolvedFixture, error) {
	typ := suite.StructType()
	if typ == nil {
		return nil, nil
	}

	var fixtures []suiteFixtureMatch
	var sharedRefs []SharedFixtureRef
	sfIdx := 0

	for i := 0; i < typ.NumFields(); i++ {
		field := typ.Field(i)
		named := pointerNamed(field)
		if named == nil {
			continue
		}
		name := named.Obj().Name()

		if strings.HasSuffix(name, "SharedFixture") {
			ref, err := r.buildSharedFixtureRef(named, sfIdx)
			if err != nil {
				return nil, err
			}
			ref.FieldName = field.Name()
			sharedRefs = append(sharedRefs, ref)
			sfIdx++
		} else if strings.HasSuffix(name, "Fixture") {
			rf, err := r.resolveFixture(named)
			if err != nil {
				return nil, err
			}
			fixtures = append(fixtures, suiteFixtureMatch{resolved: rf, fieldName: field.Name()})
		}
	}

	if len(fixtures) > 1 {
		return nil, fmt.Errorf("test suite %q embeds multiple fixtures; at most one is allowed", suite.Identifier())
	}

	if len(sharedRefs) > 0 {
		if r.result.SuiteSharedFixtures == nil {
			r.result.SuiteSharedFixtures = make(map[string][]SharedFixtureRef)
		}
		r.result.SuiteSharedFixtures[suite.Identifier()] = sharedRefs
	}

	if len(fixtures) == 0 {
		return nil, nil
	}
	suite.SetFixtureFieldName(fixtures[0].fieldName)
	return fixtures[0].resolved, nil
}

func (r *resolver) resolveFixture(named *types.Named) (*ResolvedFixture, error) {
	if rf, ok := r.resolved[named]; ok {
		return rf, nil
	}

	if r.resolving[named] {
		return nil, fmt.Errorf("cycle detected in fixture embedding: %q", named.Obj().Name())
	}
	r.resolving[named] = true
	defer delete(r.resolving, named)

	name := named.Obj().Name()
	typePkgPath := named.Obj().Pkg().Path()
	isLocal := typePkgPath == r.targetPkg.PkgPath

	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%s: fixture must be a struct type", name)
	}

	kind := gotestast.PackageFixture
	if strings.HasSuffix(name, "SharedFixture") {
		kind = gotestast.SharedFixture
	}

	pkg := r.findPackageForType(named)

	var spec *gotestast.FixtureSpec
	if isLocal {
		for _, lf := range r.localFixtures {
			if lf.Identifier() == name {
				spec = lf
				break
			}
		}
	}

	mset := types.NewMethodSet(types.NewPointer(named))
	typePkg := named.Obj().Pkg()

	rf := &ResolvedFixture{
		Kind:       kind,
		Identifier: name,
		Named:      named,
		Pkg:          pkg,
		Spec:         spec,
		BeforeAll:    mset.Lookup(typePkg, "BeforeAll") != nil,
		AfterAll:     mset.Lookup(typePkg, "AfterAll") != nil,
		BeforeEach:   mset.Lookup(typePkg, "BeforeEach") != nil,
		AfterEach:    mset.Lookup(typePkg, "AfterEach") != nil,
		HasHydrate:   mset.Lookup(typePkg, "Hydrate") != nil,
		HasDehydrate: mset.Lookup(typePkg, "Dehydrate") != nil,
		HasConfig:    detectConfigMethod(mset, typePkg, kind),
	}

	if isLocal {
		rf.QualifiedType = name
	} else {
		rf.PkgName = named.Obj().Pkg().Name()
		rf.QualifiedType = rf.PkgName + "." + name
		rf.PkgPath = typePkgPath
	}

	if !rf.BeforeAll {
		kindStr := "package fixture"
		if kind == gotestast.SharedFixture {
			kindStr = "shared fixture"
		}
		return nil, fmt.Errorf("%s %q must have a BeforeAll(ctx context.Context) error method", kindStr, name)
	}

	if kind == gotestast.PackageFixture {
		if err := r.resolvePackageFixtureFields(rf, st); err != nil {
			return nil, err
		}
	}

	r.resolved[named] = rf
	return rf, nil
}

func (r *resolver) resolvePackageFixtureFields(rf *ResolvedFixture, st *types.Struct) error {
	var parent *ResolvedFixture
	var parentFieldName string
	sfIdx := 0

	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		named := pointerNamed(field)
		if named == nil {
			continue
		}
		typeName := named.Obj().Name()

		if strings.HasSuffix(typeName, "SharedFixture") {
			sfRef, err := r.buildSharedFixtureRef(named, sfIdx)
			if err != nil {
				return err
			}
			sfRef.FieldName = field.Name()
			rf.SharedFixtures = append(rf.SharedFixtures, sfRef)
			sfIdx++
		} else if strings.HasSuffix(typeName, "Fixture") {
			if parent != nil {
				return fmt.Errorf("fixture %q embeds multiple fixtures; at most one parent is allowed", rf.Identifier)
			}
			var err error
			parent, err = r.resolveFixture(named)
			if err != nil {
				return err
			}
			parentFieldName = field.Name()
		}
	}

	if parent != nil {
		rf.Parent = parent
		rf.ParentFieldName = parentFieldName
		parent.Children = append(parent.Children, rf)
	}
	return nil
}

func (r *resolver) buildSharedFixtureRef(named *types.Named, idx int) (SharedFixtureRef, error) {
	name := named.Obj().Name()
	typePkg := named.Obj().Pkg()
	typePkgPath := typePkg.Path()
	isLocal := typePkgPath == r.targetPkg.PkgPath

	mset := types.NewMethodSet(types.NewPointer(named))
	hasHydrate := mset.Lookup(typePkg, "Hydrate") != nil
	hasDehydrate := mset.Lookup(typePkg, "Dehydrate") != nil

	qualifiedType := name
	var pkgPath string
	if !isLocal {
		qualifiedType = typePkg.Name() + "." + name
		pkgPath = typePkgPath
	}

	stateKey := typePkgPath + "." + name

	ref := SharedFixtureRef{
		LocalVar:      fmt.Sprintf("sf%d", idx),
		QualifiedType: qualifiedType,
		FieldName:     name,
		StateKey:      stateKey,
		HasHydrate:    hasHydrate,
		HasDehydrate:  hasDehydrate,
		PkgPath:       pkgPath,
	}

	r.registerSharedFixture(named)

	return ref, nil
}

func (r *resolver) registerSharedFixture(named *types.Named) {
	typePkg := named.Obj().Pkg()
	name := named.Obj().Name()
	key := typePkg.Path() + "." + name

	if _, ok := r.sharedSeen[key]; ok {
		return
	}

	mset := types.NewMethodSet(types.NewPointer(named))
	hasHydrate := mset.Lookup(typePkg, "Hydrate") != nil
	hasDehydrate := mset.Lookup(typePkg, "Dehydrate") != nil
	hasConfig := detectConfigMethod(mset, typePkg, gotestast.SharedFixture)

	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		r.sharedSeen[key] = &SharedFixtureInfo{
			Identifier: name,
			PkgPath:    typePkg.Path(),
			HasConfig:  hasConfig,
			HasHydrate: hasHydrate,
			HasDehydrate: hasDehydrate,
		}
		return
	}

	var allExported []string
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Exported() && !f.Anonymous() {
			allExported = append(allExported, f.Name())
		}
	}

	var localFields map[string]bool
	if hasHydrate {
		pkg := r.findPackageForType(named)
		if pkg != nil && len(pkg.Syntax) > 0 {
			hydrateDecl := findHydrateDecl(pkg, name)
			if hydrateDecl != nil {
				localFields = gotestast.ClassifyLocalFieldsRaw(hydrateDecl, name, pkg.Syntax, pkg.TypesInfo)
			}
		}
	}

	var transfer, local []string
	for _, fieldName := range allExported {
		if localFields[fieldName] {
			local = append(local, fieldName)
		} else {
			transfer = append(transfer, fieldName)
		}
	}

	r.sharedSeen[key] = &SharedFixtureInfo{
		Identifier:     name,
		PkgPath:        typePkg.Path(),
		HasConfig:      hasConfig,
		HasHydrate:     hasHydrate,
		HasDehydrate:   hasDehydrate,
		TransferFields: transfer,
		LocalFields:    local,
	}
}

func (r *resolver) findPackageForType(named *types.Named) *packages.Package {
	targetPath := named.Obj().Pkg().Path()
	if targetPath == r.targetPkg.PkgPath {
		return r.targetPkg
	}
	return findImportedPackage(r.targetPkg, targetPath, make(map[string]bool))
}

func findImportedPackage(pkg *packages.Package, targetPath string, visited map[string]bool) *packages.Package {
	if visited[pkg.PkgPath] {
		return nil
	}
	visited[pkg.PkgPath] = true
	for path, imp := range pkg.Imports {
		if path == targetPath {
			return imp
		}
	}
	for _, imp := range pkg.Imports {
		if found := findImportedPackage(imp, targetPath, visited); found != nil {
			return found
		}
	}
	return nil
}

func (r *resolver) findLocalSpec(rf *ResolvedFixture) *gotestast.FixtureSpec {
	if rf.Spec != nil {
		return rf.Spec
	}
	for _, lf := range r.localFixtures {
		if lf.Identifier() == rf.Identifier {
			return lf
		}
	}
	return nil
}

func findHydrateDecl(pkg *packages.Package, fixtureName string) *ast.FuncDecl {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || fd.Name.Name != "Hydrate" {
				continue
			}
			obj := pkg.TypesInfo.ObjectOf(fd.Name)
			fn, ok := obj.(*types.Func)
			if !ok {
				continue
			}
			sig, ok := fn.Type().(*types.Signature)
			if !ok || sig.Recv() == nil {
				continue
			}
			recv := sig.Recv().Type()
			if ptr, ok := recv.(*types.Pointer); ok {
				recv = ptr.Elem()
			}
			named, ok := recv.(*types.Named)
			if !ok || named.Obj().Name() != fixtureName {
				continue
			}
			return fd
		}
	}
	return nil
}

func detectConfigMethod(mset *types.MethodSet, typePkg *types.Package, kind gotestast.FixtureKind) bool {
	switch kind {
	case gotestast.PackageFixture:
		return mset.Lookup(typePkg, "FixtureConfig") != nil
	case gotestast.SharedFixture:
		return mset.Lookup(typePkg, "SharedFixtureConfig") != nil
	}
	return false
}

func pointerNamed(field *types.Var) *types.Named {
	ptr, ok := field.Type().(*types.Pointer)
	if !ok {
		return nil
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return nil
	}
	return named
}
