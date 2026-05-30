package gotestgen

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"
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
	ParentFieldName string // field name in this fixture's struct that holds the parent fixture pointer (single-parent compat)
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

	Parent         *ResolvedFixture   // single parent (backward compat)
	Parents        []*ResolvedFixture // all parent fixtures
	ParentFields   map[*ResolvedFixture]string // parent fixture → field name in this fixture's struct
	Children       []*ResolvedFixture
	SharedFixtures []SharedFixtureRef
	ChildSuites    []*gotestast.TestSuiteSpec
}

// FixtureFieldBinding maps a fixture identifier to its field name in a suite struct.
type FixtureFieldBinding struct {
	FixtureIdentifier string
	FieldName         string
}

// ResolveResult is the output of fixture resolution for a target package.
type ResolveResult struct {
	RootFixtures                   []*ResolvedFixture
	AllFixtures                    []*ResolvedFixture       // topologically sorted, all fixtures
	RequiredSharedFixtures         []SharedFixtureInfo      // deduplicated, for setup subprocess
	FixtureBound                   []*gotestast.TestSuiteSpec
	Standalone                     []*gotestast.TestSuiteSpec
	SuiteSharedFixtures            map[string][]SharedFixtureRef    // suite identifier → direct shared fixture refs
	SuiteFixtureFields             map[string][]FixtureFieldBinding // suite identifier → fixture→field bindings
	SuiteRequiredSharedFixtureKeys map[string][]string              // suite identifier → all required state keys (transitive)
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
		if suite.IsGenericAlias() && suite.IsPxTestSuite() {
			return nil, fmt.Errorf("generic alias suite %q must not be in an external test package (pxtest); move it to the internal test file", suite.Identifier())
		}

		fixtures, err := r.resolveFixturesForSuite(suite)
		if err != nil {
			return nil, err
		}
		if len(fixtures) > 0 {
			if result.SuiteFixtureFields == nil {
				result.SuiteFixtureFields = make(map[string][]FixtureFieldBinding)
			}
			var bindings []FixtureFieldBinding
			for _, fm := range fixtures {
				fm.resolved.ChildSuites = append(fm.resolved.ChildSuites, suite)
				bindings = append(bindings, FixtureFieldBinding{
					FixtureIdentifier: fm.resolved.Identifier,
					FieldName:         fm.fieldName,
				})
			}
			result.SuiteFixtureFields[suite.Identifier()] = bindings
			suite.SetFixture(r.findLocalSpec(fixtures[0].resolved))
			suite.SetFixtureFieldName(fixtures[0].fieldName)
			result.FixtureBound = append(result.FixtureBound, suite)
		} else {
			result.Standalone = append(result.Standalone, suite)
		}
	}

	// Collect unique root fixtures (fixtures with no parents)
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

	// Topological sort all fixtures
	allFixtures, err := topologicalSort(r.resolved)
	if err != nil {
		return nil, err
	}
	result.AllFixtures = allFixtures

	// Collect deduplicated shared fixtures in deterministic topological order
	// (dependencies before dependents, ties broken by identifier).
	result.RequiredSharedFixtures = topoSortSharedFixtures(r.sharedSeen)

	// Compute per-suite transitive shared fixture keys
	suiteKeys := make(map[string][]string)
	sfInfoByKey := make(map[string]*SharedFixtureInfo)
	for i := range result.RequiredSharedFixtures {
		sf := &result.RequiredSharedFixtures[i]
		sfInfoByKey[sf.PkgPath+"."+sf.Identifier] = sf
	}

	var collectTransitive func(key string, seen map[string]bool)
	collectTransitive = func(key string, seen map[string]bool) {
		if seen[key] {
			return
		}
		seen[key] = true
		if info, ok := sfInfoByKey[key]; ok {
			for _, dep := range info.Dependencies {
				collectTransitive(dep, seen)
			}
		}
	}

	for _, suite := range suites {
		id := suite.Identifier()
		seen := make(map[string]bool)

		// From direct suite shared fixture refs
		if refs, ok := result.SuiteSharedFixtures[id]; ok {
			for _, ref := range refs {
				collectTransitive(ref.StateKey, seen)
			}
		}

		// From fixture tree shared fixtures
		if bindings, ok := result.SuiteFixtureFields[id]; ok {
			for _, b := range bindings {
				for _, rf := range result.AllFixtures {
					if rf.Identifier == b.FixtureIdentifier {
						for _, sf := range rf.SharedFixtures {
							collectTransitive(sf.StateKey, seen)
						}
					}
				}
			}
		}

		if len(seen) > 0 {
			var keys []string
			for k := range seen {
				keys = append(keys, k)
			}
			suiteKeys[id] = keys
		}
	}
	if len(suiteKeys) > 0 {
		result.SuiteRequiredSharedFixtureKeys = suiteKeys
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

func (r *resolver) resolveFixturesForSuite(suite *gotestast.TestSuiteSpec) ([]suiteFixtureMatch, error) {
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

	if len(sharedRefs) > 0 {
		if r.result.SuiteSharedFixtures == nil {
			r.result.SuiteSharedFixtures = make(map[string][]SharedFixtureRef)
		}
		r.result.SuiteSharedFixtures[suite.Identifier()] = sharedRefs
	}

	return fixtures, nil
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

	baseName := named.Obj().Name()
	identifier := fixtureIdentifier(named)
	typePkgPath := named.Obj().Pkg().Path()
	isLocal := typePkgPath == r.targetPkg.PkgPath

	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%s: fixture must be a struct type", identifier)
	}

	kind := gotestast.PackageFixture
	if strings.HasSuffix(baseName, "SharedFixture") {
		kind = gotestast.SharedFixture
	}

	pkg := r.findPackageForType(named)

	var spec *gotestast.FixtureSpec
	if isLocal {
		for _, lf := range r.localFixtures {
			if lf.Identifier() == baseName {
				spec = lf
				break
			}
		}
	}

	mset := types.NewMethodSet(types.NewPointer(named))
	typePkg := named.Obj().Pkg()

	rf := &ResolvedFixture{
		Kind:       kind,
		Identifier: identifier,
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
		rf.QualifiedType = fixtureQualifiedType(named, "")
	} else {
		rf.PkgName = named.Obj().Pkg().Name()
		rf.QualifiedType = fixtureQualifiedType(named, rf.PkgName)
		rf.PkgPath = typePkgPath
		rf.Identifier = rf.PkgName + "_" + identifier
	}

	if !rf.BeforeAll {
		kindStr := "package fixture"
		if kind == gotestast.SharedFixture {
			kindStr = "shared fixture"
		}
		return nil, fmt.Errorf("%s %q must have a BeforeAll(ctx context.Context) error method", kindStr, identifier)
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
			parent, err := r.resolveFixture(named)
			if err != nil {
				return err
			}
			rf.Parents = append(rf.Parents, parent)
			if rf.ParentFields == nil {
				rf.ParentFields = make(map[*ResolvedFixture]string)
			}
			rf.ParentFields[parent] = field.Name()
			parent.Children = append(parent.Children, rf)
		}
	}

	if len(rf.Parents) > 0 {
		rf.Parent = rf.Parents[0]
		rf.ParentFieldName = rf.ParentFields[rf.Parents[0]]
	}
	return nil
}

func isInternalPkgPath(pkgPath string) bool {
	return strings.HasPrefix(pkgPath, "internal/") ||
		strings.HasSuffix(pkgPath, "/internal") ||
		strings.Contains(pkgPath, "/internal/")
}

func (r *resolver) buildSharedFixtureRef(named *types.Named, idx int) (SharedFixtureRef, error) {
	identifier := fixtureIdentifier(named)
	typePkg := named.Obj().Pkg()
	typePkgPath := typePkg.Path()

	isLocal := typePkgPath == r.targetPkg.PkgPath

	if !isLocal && isInternalPkgPath(typePkgPath) {
		return SharedFixtureRef{}, fmt.Errorf(
			"shared fixture %q is in an internal package (%s); "+
				"shared fixtures must live in a non-internal package so the setup subprocess can import them",
			identifier, typePkgPath,
		)
	}

	mset := types.NewMethodSet(types.NewPointer(named))
	hasHydrate := mset.Lookup(typePkg, "Hydrate") != nil
	hasDehydrate := mset.Lookup(typePkg, "Dehydrate") != nil

	var qualifiedType, pkgPath string
	if isLocal {
		qualifiedType = fixtureQualifiedType(named, "")
	} else {
		qualifiedType = fixtureQualifiedType(named, typePkg.Name())
		pkgPath = typePkgPath
	}

	stateKey := typePkgPath + "." + identifier

	sfIdentifier := identifier
	if !isLocal {
		sfIdentifier = typePkg.Name() + "_" + identifier
	}

	ref := SharedFixtureRef{
		LocalVar:      fmt.Sprintf("sf%d", idx),
		QualifiedType: qualifiedType,
		FieldName:     named.Obj().Name(),
		StateKey:      stateKey,
		Identifier:    sfIdentifier,
		HasHydrate:    hasHydrate,
		HasDehydrate:  hasDehydrate,
		PkgPath:       pkgPath,
	}

	if err := r.registerSharedFixture(named); err != nil {
		return SharedFixtureRef{}, err
	}

	return ref, nil
}

func (r *resolver) registerSharedFixture(named *types.Named) error {
	typePkg := named.Obj().Pkg()
	identifier := fixtureIdentifier(named)
	baseName := named.Obj().Name()
	key := typePkg.Path() + "." + identifier

	if _, ok := r.sharedSeen[key]; ok {
		return nil
	}

	mset := types.NewMethodSet(types.NewPointer(named))
	hasHydrate := mset.Lookup(typePkg, "Hydrate") != nil
	hasDehydrate := mset.Lookup(typePkg, "Dehydrate") != nil
	hasConfig := detectConfigMethod(mset, typePkg, gotestast.SharedFixture)

	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		r.sharedSeen[key] = &SharedFixtureInfo{
			Identifier:   identifier,
			PkgPath:      typePkg.Path(),
			HasConfig:    hasConfig,
			HasHydrate:   hasHydrate,
			HasDehydrate: hasDehydrate,
		}
		return nil
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
			hydrateDecl := findHydrateDecl(pkg, baseName)
			if hydrateDecl != nil {
				localFields = gotestast.ClassifyLocalFieldsRaw(hydrateDecl, baseName, pkg.Syntax, pkg.TypesInfo)
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

	// Detect shared fixture pointer fields as dependencies.
	var deps []string
	depFields := make(map[string]bool)
	depFieldMap := make(map[string]string) // dep state key → field name
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		depNamed := pointerNamed(f)
		if depNamed == nil {
			continue
		}
		depName := depNamed.Obj().Name()
		if strings.HasSuffix(depName, "SharedFixture") {
			depKey := depNamed.Obj().Pkg().Path() + "." + fixtureIdentifier(depNamed)
			deps = append(deps, depKey)
			depFields[f.Name()] = true
			depFieldMap[depKey] = f.Name()
			if err := r.registerSharedFixture(depNamed); err != nil {
				return err
			}
		}
	}

	// Remove dependency fields from transfer (they are pointers and not JSON-serializable).
	if len(depFields) > 0 {
		filtered := transfer[:0]
		for _, name := range transfer {
			if !depFields[name] {
				filtered = append(filtered, name)
			}
		}
		transfer = filtered
	}

	for _, fieldName := range transfer {
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			if f.Name() == fieldName {
				if err := validateTransferFieldType(identifier, f); err != nil {
					return err
				}
				break
			}
		}
	}

	r.sharedSeen[key] = &SharedFixtureInfo{
		Identifier:       identifier,
		PkgPath:          typePkg.Path(),
		HasConfig:        hasConfig,
		HasHydrate:       hasHydrate,
		HasDehydrate:     hasDehydrate,
		TransferFields:   transfer,
		LocalFields:      local,
		Dependencies:     deps,
		DependencyFields: depFieldMap,
	}
	return nil
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

func validateTransferFieldType(fixtureName string, field *types.Var) error {
	if reason := nonSerializable(field.Type()); reason != "" {
		return fmt.Errorf("shared fixture %q: transfer field %q has non-JSON-serializable type %s (%s)", fixtureName, field.Name(), field.Type(), reason)
	}
	return nil
}

func nonSerializable(t types.Type) string {
	switch u := t.Underlying().(type) {
	case *types.Chan:
		return "channel"
	case *types.Signature:
		return "function"
	case *types.Array:
		return nonSerializable(u.Elem())
	case *types.Slice:
		return nonSerializable(u.Elem())
	case *types.Map:
		if r := nonSerializable(u.Key()); r != "" {
			return r + " in map key"
		}
		return nonSerializable(u.Elem())
	case *types.Pointer:
		return nonSerializable(u.Elem())
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			f := u.Field(i)
			if !f.Exported() {
				continue
			}
			if r := nonSerializable(f.Type()); r != "" {
				return r + " in field " + f.Name()
			}
		}
	}
	return ""
}

// topoSortSharedFixtures returns shared fixtures sorted in topological order
// (dependencies before dependents), with ties broken by identifier for stability.
func topoSortSharedFixtures(seen map[string]*SharedFixtureInfo) []SharedFixtureInfo {
	if len(seen) == 0 {
		return nil
	}

	// Collect all keys and sort for deterministic tie-breaking.
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	inDegree := make(map[string]int, len(keys))
	for _, k := range keys {
		inDegree[k] = len(seen[k].Dependencies)
	}

	// Adjacency: depKey → list of nodes that depend on it.
	children := make(map[string][]string)
	for _, k := range keys {
		for _, dep := range seen[k].Dependencies {
			children[dep] = append(children[dep], k)
		}
	}

	// Build initial queue of zero-degree nodes (sorted for determinism).
	var queue []string
	for _, k := range keys {
		if inDegree[k] == 0 {
			queue = append(queue, k)
		}
	}

	var result []SharedFixtureInfo
	for len(queue) > 0 {
		// Pop first element.
		node := queue[0]
		queue = queue[1:]
		result = append(result, *seen[node])
		// Reduce in-degree of dependents.
		deps := children[node]
		sort.Strings(deps)
		for _, child := range deps {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
				sort.Strings(queue) // keep queue sorted for determinism
			}
		}
	}

	return result
}

func topologicalSort(resolved map[*types.Named]*ResolvedFixture) ([]*ResolvedFixture, error) {
	inDegree := make(map[*ResolvedFixture]int)
	var all []*ResolvedFixture
	for _, rf := range resolved {
		if rf.Kind != gotestast.PackageFixture {
			continue
		}
		all = append(all, rf)
		if _, ok := inDegree[rf]; !ok {
			inDegree[rf] = 0
		}
		for _, child := range rf.Children {
			inDegree[child]++
		}
	}

	var queue []*ResolvedFixture
	for _, rf := range all {
		if inDegree[rf] == 0 {
			queue = append(queue, rf)
		}
	}

	var sorted []*ResolvedFixture
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)
		for _, child := range node.Children {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if len(sorted) != len(all) {
		return nil, fmt.Errorf("cycle detected in fixture dependency graph")
	}
	return sorted, nil
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

func fixtureIdentifier(named *types.Named) string {
	name := named.Obj().Name()
	if targs := named.TypeArgs(); targs != nil && targs.Len() > 0 {
		parts := make([]string, targs.Len())
		for i := range targs.Len() {
			arg := targs.At(i)
			if n, ok := arg.(*types.Named); ok {
				parts[i] = n.Obj().Name()
			} else {
				parts[i] = arg.String()
			}
		}
		name += "_" + strings.Join(parts, "_")
	}
	return name
}

func fixtureQualifiedType(named *types.Named, pkgName string) string {
	name := named.Obj().Name()
	if targs := named.TypeArgs(); targs != nil && targs.Len() > 0 {
		parts := make([]string, targs.Len())
		for i := range targs.Len() {
			arg := targs.At(i)
			if n, ok := arg.(*types.Named); ok {
				argPkg := n.Obj().Pkg()
				if argPkg != nil && argPkg.Name() != pkgName {
					parts[i] = argPkg.Name() + "." + n.Obj().Name()
				} else {
					parts[i] = n.Obj().Name()
				}
			} else {
				parts[i] = arg.String()
			}
		}
		name += "[" + strings.Join(parts, ", ") + "]"
	}
	if pkgName != "" {
		return pkgName + "." + name
	}
	return name
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
