package gotestgen_test

import (
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// FuzzFanTestSuite covers fuzz-call discovery and target emission — the
// failure mode that matters is silently generating wrong code, so these
// tests assert on emitted source and on rejection messages, and
// fuzzfan_compile_test.go type-checks and round-trips the real output.
type FuzzFanTestSuite struct{}

// collectCalls is the discovery half of the pipeline, run over one of the
// shared testdata/sources fixtures.
func collectCalls(t *gotest.T, fixture string) (*packages.Package, []gotestast.FuzzCall) {
	pkg := gotestgen.ExportMustTestPkg(t.T(), fixture)
	c := gotestgen.NewCollector()
	result := c.CollectSuiteSpecs(pkg)
	gotest.Empty(t, result.Errs, "collection errors: %v", result.Errs)
	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)
	calls, err := gotestast.CollectFuzzCalls(pkg, spec.EffectiveTestSuites)
	gotest.NoError(t, err)
	return pkg, calls
}

func (s *FuzzFanTestSuite) TestCollectFuzzCalls(t *gotest.T) {
	t.It("reads the callback type of every f.Fuzz call", func(it *gotest.T) {
		_, calls := collectCalls(it, "TestFuzzCodec_StructTarget")

		gotest.Len(it, calls, 2)

		byFunc := map[string]gotestast.FuzzCall{}
		for _, c := range calls {
			byFunc[c.FuncName] = c
		}

		create, ok := byFunc["FuzzStructFuzzTestSuite_FuzzCreate"]
		gotest.True(it, ok, "expected an entry for FuzzCreate, got %v", byFunc)
		gotest.Len(it, create.Types, 1)
		gotest.Regexp(it, `\.Request$`, types.TypeString(create.Types[0], nil))

		native, ok := byFunc["FuzzStructFuzzTestSuite_FuzzNative"]
		gotest.True(it, ok, "expected an entry for FuzzNative, got %v", byFunc)
		gotest.Equal(it, "string", types.TypeString(native.Types[0], nil))
	})

	t.It("returns nothing for a package with no fuzz methods", func(it *gotest.T) {
		_, calls := collectCalls(it, "TestRenderer_FixtureWithChildSuite")
		gotest.Empty(it, calls)
	})
}

// buildTargets runs the full discovery + emission pipeline over a shared
// testdata/sources fixture.
func buildTargets(t *gotest.T, fixture string) (*gotestgen.FuzzTargetSet, error) {
	pkg := gotestgen.ExportMustTestPkg(t.T(), fixture)
	c := gotestgen.NewCollector()
	result := c.CollectSuiteSpecs(pkg)
	gotest.Empty(t, result.Errs, "collection errors: %v", result.Errs)
	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)
	return gotestgen.BuildFuzzTargets(pkg, spec.EffectiveTestSuites)
}

// build is buildTargets for tests that only care about the successful-build
// shape — it fails the test immediately on any generation error.
func (s *FuzzFanTestSuite) build(t *gotest.T, fixture string) *gotestgen.FuzzTargetSet {
	set, err := buildTargets(t, fixture)
	gotest.NoError(t, err)
	return set
}

func targetExpr(set *gotestgen.FuzzTargetSet, funcName string) string {
	for _, ref := range set.Targets {
		if ref.FuncName == funcName {
			return ref.Expr
		}
	}
	return ""
}

func (s *FuzzFanTestSuite) TestStructTarget(t *gotest.T) {
	t.It("emits one target per fuzz method, pass-through methods included", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Len(it, set.Targets, 2)
		gotest.Equal(it, `&gotest.FuzzTarget{Signature: "func(*gotest.T, Request)", Register: ƒ_fuzzreg_v1_FuzzStructFuzzTestSuite_FuzzCreate, Explode: ƒ_fuzzexp_v1_FuzzStructFuzzTestSuite_FuzzCreate}`,
			targetExpr(set, "FuzzStructFuzzTestSuite_FuzzCreate"))
		gotest.Equal(it, `&gotest.FuzzTarget{Signature: "func(*gotest.T, string)", Register: ƒ_fuzzreg_v1_FuzzStructFuzzTestSuite_FuzzNative, Explode: ƒ_fuzzexp_v1_FuzzStructFuzzTestSuite_FuzzNative}`,
			targetExpr(set, "FuzzStructFuzzTestSuite_FuzzNative"))
	})

	t.It("fans fields in declaration order with the leaf encoding policy", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		// Email string → string leaf; Age int → []byte leaf; Prio Priority
		// (named int) → []byte leaf; Tags []string → hybrid []byte leaf;
		// Home *Address → bool nil-flag + Street string + Zip []byte.
		gotest.Contains(it, set.Source, "func ƒ_fuzzin_v1_Request(ƒ0 string, ƒ1 []byte, ƒ2 []byte, ƒ3 []byte, ƒ4 bool, ƒ5 string, ƒ6 []byte) Request {")
		gotest.Contains(it, set.Source, "return Request{Email: ƒ0, Age: ƒ_fuzzin_v1_int(ƒ1), Prio: ƒ_fuzzin_v1_Priority(ƒ2), Tags: ƒ_fuzzin_v1_slice_string(ƒ3), Home: ƒ_fuzzin_v1_ptr_Address(ƒ4, ƒ5, ƒ6)}")
		gotest.Contains(it, set.Source, "func ƒ_fuzzin_v1_int(ƒ0 []byte) int {\n\treturn gotestfuzz.LeafInt(ƒ0)\n}")
		gotest.Contains(it, set.Source, "func ƒ_fuzzin_v1_Priority(ƒ0 []byte) Priority {\n\treturn Priority(gotestfuzz.LeafInt(ƒ0))\n}")
		gotest.Contains(it, set.Source, "return []any{gotestfuzz.LeafBytesInt(int(ƒv))}")
		gotest.True(it, set.NeedsRuntime)
	})

	t.It("binds the callback by its exact type and registers through a direct (*testing.F).Fuzz call", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Contains(it, set.Source, "func ƒ_fuzzreg_v1_FuzzStructFuzzTestSuite_FuzzCreate(ƒf *testing.F, ƒfn any, ƒeach gotest.FuzzEach) bool {\n\tƒrun, ok := ƒfn.(func(*gotest.T, Request))\n\tif !ok {\n\t\treturn false\n\t}")
		gotest.Contains(it, set.Source, "ƒf.Fuzz(func(ƒt *testing.T, ƒ0 string, ƒ1 []byte, ƒ2 []byte, ƒ3 []byte, ƒ4 bool, ƒ5 string, ƒ6 []byte) {\n\t\tƒa0 := ƒ_fuzzin_v1_Request(ƒ0, ƒ1, ƒ2, ƒ3, ƒ4, ƒ5, ƒ6)")
		gotest.Contains(it, set.Source, "ƒeach(ƒt, func() string { return ƒ_fuzzlits_v1_Request(ƒa0) }, func(ƒtt *gotest.T) { ƒrun(ƒtt, ƒa0) })")
	})

	t.It("explodes a typed seed after checking its arity and each position's type", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Contains(it, set.Source, "func ƒ_fuzzexp_v1_FuzzStructFuzzTestSuite_FuzzCreate(ƒseed []any) ([]any, error) {\n\tif err := gotest.SeedArity(ƒseed, 1); err != nil {\n\t\treturn nil, err\n\t}\n\tƒa0, err := gotest.SeedArg[Request](ƒseed, 0)")
		gotest.Contains(it, set.Source, "ƒo = append(ƒo, ƒ_fuzzout_v1_Request(ƒa0)...)\n\treturn ƒo, nil")
	})

	t.It("unrolls a pointer as a nil-flag plus the pointee's leaves, exploding nil to full arity", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Contains(it, set.Source, "func ƒ_fuzzin_v1_ptr_Address(ƒ0 bool, ƒ1 string, ƒ2 []byte) *Address {")
		gotest.Contains(it, set.Source, "if !ƒ0 {\n\t\treturn nil\n\t}\n\tƒx := ƒ_fuzzin_v1_Address(ƒ1, ƒ2)\n\treturn &ƒx")
		gotest.Contains(it, set.Source, "if ƒv == nil {\n\t\tvar ƒz Address\n\t\treturn append([]any{false}, ƒ_fuzzout_v1_Address(ƒz)...)\n\t}\n\treturn append([]any{true}, ƒ_fuzzout_v1_Address(*ƒv)...)")
	})

	t.It("rides a slice of strings as one hybrid leaf through the total mini-codec", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Contains(it, set.Source, "func ƒ_fuzzdec_v1_slice_string(ƒb []byte) []string {\n\tƒr := gotestfuzz.NewReader(ƒb)\n\treturn ƒ_fuzzread_v1_slice_string(ƒr)\n}")
		gotest.Contains(it, set.Source, "func ƒ_fuzzenc_v1_slice_string(ƒv []string) []byte {")
		gotest.Contains(it, set.Source, "func ƒ_fuzzread_v1_slice_string(ƒr *gotestfuzz.Reader) []string {")
	})

	t.It("collapses an empty pass-through []byte field to nil on fan-in", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_DeclaredKinds")
		// The declared-position []byte in FuzzMixed3 is top-level: untouched.
		gotest.Contains(it, set.Source, "ƒa0 := ƒ0\n\t\tƒa1 := ƒ_fuzzin_v1_uint16(ƒ1)\n\t\tƒa2 := ƒ2")
		set = s.build(it, "TestFuzzCodec_StructTarget")
		gotest.NotContains(it, set.Source, "LeafBytes(ƒ0)", "no []byte field in Request")
	})

	t.It("reports every target's corpus shape, pass-through targets included", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Equal(it, []string{"string", "[]byte", "[]byte", "[]byte", "bool", "string", "[]byte"}, set.ParamsByFunc["FuzzStructFuzzTestSuite_FuzzCreate"])
		gotest.Equal(it, []string{"string"}, set.ParamsByFunc["FuzzStructFuzzTestSuite_FuzzNative"])
	})

	t.It("reports no import beyond the runtime for same-package types", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Empty(it, set.PkgPaths)
	})

	t.It("binds a pass-through-only package without the leaf runtime", func(it *gotest.T) {
		set := s.build(it, "TestCollector_FuzzMethod")
		gotest.NotZero(it, set)
		gotest.NotEmpty(it, set.Targets)
		gotest.Contains(it, set.Source, "ƒfn.(func(*gotest.T, string))")
		gotest.False(it, set.NeedsRuntime)
		gotest.NotEmpty(it, set.ParamsByFunc)
	})

	t.It("returns nothing at all for a package without fuzz targets", func(it *gotest.T) {
		set := s.build(it, "TestRenderer_FixtureWithChildSuite")
		gotest.Zero(it, set)
	})
}

func (s *FuzzFanTestSuite) TestDeclaredKinds(t *gotest.T) {
	t.It("fans a bare declared int so it rides as a []byte leaf", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_DeclaredKinds")
		gotest.Contains(it, set.Source, "ƒrun, ok := ƒfn.(func(*gotest.T, int))")
		gotest.Contains(it, set.Source, "ƒf.Fuzz(func(ƒt *testing.T, ƒ0 []byte) {\n\t\tƒa0 := ƒ_fuzzin_v1_int(ƒ0)")
		gotest.Equal(it, []string{"[]byte"}, set.ParamsByFunc["FuzzDeclaredFuzzTestSuite_FuzzInt"])
	})

	t.It("passes an all-native two-value callback straight through", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_DeclaredKinds")
		gotest.Contains(it, set.Source, "ƒrun, ok := ƒfn.(func(*gotest.T, string, string))")
		gotest.Contains(it, set.Source, "ƒf.Fuzz(func(ƒt *testing.T, ƒ0 string, ƒ1 string) {\n\t\tƒa0 := ƒ0\n\t\tƒa1 := ƒ1")
		gotest.Equal(it, []string{"string", "string"}, set.ParamsByFunc["FuzzDeclaredFuzzTestSuite_FuzzTwoStrings"])
	})

	t.It("fans a mixed three-value callback position by position, with a tuple explode and literal", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_DeclaredKinds")
		gotest.Contains(it, set.Source, "ƒrun, ok := ƒfn.(func(*gotest.T, string, uint16, []byte))")
		gotest.Contains(it, set.Source, "ƒf.Fuzz(func(ƒt *testing.T, ƒ0 string, ƒ1 []byte, ƒ2 []byte) {")
		gotest.Contains(it, set.Source, "func ƒ_fuzzexp_v1_FuzzDeclaredFuzzTestSuite_FuzzMixed3(ƒseed []any) ([]any, error) {\n\tif err := gotest.SeedArity(ƒseed, 3); err != nil {")
		gotest.Contains(it, set.Source, "ƒa1, err := gotest.SeedArg[uint16](ƒseed, 1)")
		gotest.Contains(it, set.Source, "ƒo = append(ƒo, []any{ƒa0}...)")
		gotest.Contains(it, set.Source, "ƒo = append(ƒo, ƒ_fuzzout_v1_uint16(ƒa1)...)")
		gotest.Contains(it, set.Source, "func ƒ_fuzzlits_v1_string_uint16_slice_byte(ƒa0 string, ƒa1 uint16, ƒa2 []byte) string {")
		gotest.Contains(it, set.Source, `strconv.Quote(string(ƒa0)) + ", " + "uint16(" + strconv.FormatUint(uint64(ƒa1), 10) + ")" + ", " + ƒ_fuzzlit_v1_slice_byte(ƒa2)`)
		gotest.Contains(it, set.Source, "ƒeach(ƒt, func() string { return ƒ_fuzzlits_v1_string_uint16_slice_byte(ƒa0, ƒa1, ƒa2) }, func(ƒtt *gotest.T) { ƒrun(ƒtt, ƒa0, ƒa1, ƒa2) })")
		gotest.Equal(it, []string{"string", "[]byte", "[]byte"}, set.ParamsByFunc["FuzzDeclaredFuzzTestSuite_FuzzMixed3"])
		gotest.Equal(it, `&gotest.FuzzTarget{Signature: "func(*gotest.T, string, uint16, []byte)", Register: ƒ_fuzzreg_v1_FuzzDeclaredFuzzTestSuite_FuzzMixed3, Explode: ƒ_fuzzexp_v1_FuzzDeclaredFuzzTestSuite_FuzzMixed3}`,
			targetExpr(set, "FuzzDeclaredFuzzTestSuite_FuzzMixed3"))
	})

	t.It("accepts a struct beside a native value in one callback", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_MultiArgStruct")
		gotest.Len(it, set.Targets, 1)
		gotest.Contains(it, set.Targets[0].Expr, `Signature: "func(*gotest.T, Pair, int)"`)
		gotest.Contains(it, set.Source, "ƒf.Fuzz(func(ƒt *testing.T, ƒ0 []byte, ƒ1 []byte, ƒ2 []byte) {\n\t\tƒa0 := ƒ_fuzzin_v1_Pair(ƒ0, ƒ1)\n\t\tƒa1 := ƒ_fuzzin_v1_int(ƒ2)")
	})
}

func (s *FuzzFanTestSuite) TestArrays(t *gotest.T) {
	t.It("rides a byte array as one padded []byte leaf", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_Arrays")
		gotest.Contains(it, set.Source, "func ƒ_fuzzin_v1_arr16_byte(ƒ0 []byte) [16]byte {\n\tvar ƒa [16]byte\n\tcopy(ƒa[:], ƒ0)\n\treturn ƒa\n}")
		gotest.Contains(it, set.Source, "return []any{append([]byte(nil), ƒv[:]...)}")
	})

	t.It("fans a small array element-wise and a large one as a hybrid leaf", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_Arrays")
		gotest.Contains(it, set.Source, "func ƒ_fuzzin_v1_arr3_int8(ƒ0 []byte, ƒ1 []byte, ƒ2 []byte) [3]int8 {\n\treturn [3]int8{ƒ_fuzzin_v1_int8(ƒ0), ƒ_fuzzin_v1_int8(ƒ1), ƒ_fuzzin_v1_int8(ƒ2)}")
		gotest.Contains(it, set.Source, "func ƒ_fuzzin_v1_arr64_int8(ƒ0 []byte) [64]int8 {\n\treturn ƒ_fuzzdec_v1_arr64_int8(ƒ0)")
		gotest.Equal(it, []string{"[]byte", "[]byte", "[]byte", "[]byte", "[]byte"}, set.ParamsByFunc["FuzzArrayFuzzTestSuite_FuzzPacket"])
	})

	t.It("lets an empty nested struct contribute no leaves", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_Arrays")
		gotest.Contains(it, set.Source, "Empty: ƒ_fuzzin_v1_struct()")
	})
}

// TestCrossPackageTypes covers the shape every external (pxtest) fuzz target
// has: the type under fuzz lives in another package, so it must be emitted
// qualified AND its import path reported, or the generated file references a
// package it never imports and no consuming project compiles.
func (s *FuzzFanTestSuite) TestCrossPackageTypes(t *gotest.T) {
	t.It("reports the import path of every package the emitted source references", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_CrossPackage")
		gotest.Equal(it, []string{"testpkg/TestFuzzCodec_CrossDep"}, set.PkgPaths)
	})

	t.It("qualifies the foreign type but not the local one", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_CrossPackage")
		gotest.Len(it, set.Targets, 1)
		gotest.Contains(it, set.Targets[0].Expr, `Signature: "func(*gotest.T, Envelope)"`, "the fuzzed type is local, so it stays unqualified")
		gotest.Contains(it, set.Source, "func ƒ_fuzzin_v1_crossdep_Setting(")
		gotest.Contains(it, set.Source, "func ƒ_fuzzin_v1_ptr_crossdep_Setting(ƒ0 bool,")
	})

	t.It("qualifies a cross-package named basic used as a struct field", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_CrossPackage")
		gotest.Contains(it, set.Source, `"crossdep.ID(" + strconv.Quote(string(ƒv.Tag)) + ")"`)
		gotest.NotContains(it, set.Source, `"ID(" + strconv.Quote`,
			"the bare identifier is out of scope outside the crossdep package")
	})

	t.It("qualifies a cross-package named basic fuzzed directly, with no enclosing struct", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_CrossPackageBasic")
		gotest.Len(it, set.Targets, 1)
		gotest.Contains(it, set.Targets[0].Expr, `Signature: "func(*gotest.T, crossdep.ID)"`)
		gotest.Contains(it, set.Source, "return ƒ_fuzzlits_v1_crossdep_ID(ƒa0)")
		gotest.Contains(it, set.Source, `return "crossdep.ID(" + strconv.Quote(string(ƒa0)) + ")"`)
	})
}

// TestAliasDeduplication pins that an alias and its target share one fan-in:
// they are the same type, so two targets over them emit one set of
// fan functions and their own bindings.
func (s *FuzzFanTestSuite) TestAliasDeduplication(t *gotest.T) {
	t.It("emits one fan-in for an alias and its target, and a binding per method", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_AliasTarget")
		gotest.Len(it, set.Targets, 2, "one binding per fuzz method")
		gotest.Contains(it, set.Targets[0].Expr, `Signature: "func(*gotest.T, Inner)"`)
		gotest.Contains(it, set.Targets[1].Expr, `Signature: "func(*gotest.T, Inner)"`)
		gotest.Equal(it, 1, strings.Count(set.Source, "func ƒ_fuzzin_v1_Inner("))
		gotest.NotContains(it, set.Source, "ƒ_fuzzin_v1_AliasOf")
	})
}

func (s *FuzzFanTestSuite) TestLiteralFuncs(t *gotest.T) {
	t.It("emits a literal function for a struct target", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Contains(it, set.Source, "func ƒ_fuzzlit_v1_Request(ƒv Request) string {")
		gotest.Contains(it, set.Source, `strconv.Quote(`)
	})

	t.It("emits a literal function for a pointer-to-basic field, using the slice-index form", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_PtrBasicField")
		gotest.Contains(it, set.Source, "ƒeach(ƒt, func() string { return ƒ_fuzzlits_v1_", "a *int field now has a self-contained literal form")
		gotest.Contains(it, set.Source, `"&[]int{"`,
			"a non-nil *int renders as the addressable slice-index form, since \"&5\" is not valid Go")
		gotest.Contains(it, set.Source, `return "nil"`,
			"a nil *int still renders as the bare nil literal")
	})

	t.It("inlines the literal of a bare named-basic target into the tuple literal", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_NamedBasicTarget")
		gotest.Contains(it, set.Source, "return ƒ_fuzzlits_v1_Level(ƒa0)")
		gotest.Contains(it, set.Source, "func ƒ_fuzzlits_v1_Level(ƒa0 Level) string {")
		gotest.Contains(it, set.Source, `return "Level(" + strconv.FormatInt(int64(ƒa0), 10) + ")"`)
	})
}

func (s *FuzzFanTestSuite) TestRejections(t *gotest.T) {
	t.It("rejects an unexported field, naming it and the alternative", func(it *gotest.T) {
		_, err := buildTargets(it, "TestFuzzCodec_UnexportedField")
		gotest.ErrorContains(it, err, "FuzzUnexportedFuzzTestSuite_FuzzGuarded")
		gotest.ErrorContains(it, err, "Guarded.mu")
		gotest.ErrorContains(it, err, "unexported fields cannot be set")
		gotest.ErrorContains(it, err, "fuzz the constructor's input")
	})

	t.It("rejects a map field, pointing at the slice-of-pairs workaround", func(it *gotest.T) {
		_, err := buildTargets(it, "TestFuzzCodec_MapField")
		gotest.ErrorContains(it, err, "WithMap.Headers")
		gotest.ErrorContains(it, err, "slice of key/value pairs")
	})

	t.It("rejects an interface field", func(it *gotest.T) {
		_, err := buildTargets(it, "TestFuzzCodec_InterfaceField")
		gotest.ErrorContains(it, err, "WithAny.Payload")
		gotest.ErrorContains(it, err, "no value can be synthesized")
	})

	t.It("rejects a recursive type rather than emitting an unbounded fan", func(it *gotest.T) {
		_, err := buildTargets(it, "TestFuzzCodec_RecursiveType")
		gotest.ErrorContains(it, err, "recursive")
	})

	t.It("rejects a target with no fuzzable leaves", func(it *gotest.T) {
		_, err := buildTargets(it, "TestFuzzFan_NoLeaves")
		gotest.ErrorContains(it, err, "FuzzNoLeavesFuzzTestSuite_FuzzMarker")
		gotest.ErrorContains(it, err, "no fuzzable leaves")
	})
}
