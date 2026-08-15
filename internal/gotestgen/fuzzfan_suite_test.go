package gotestgen_test

import (
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// FuzzFanTestSuite covers fuzz-argument discovery and fan emission — the
// failure mode that matters is silently generating wrong code, so these
// tests assert on emitted source and on rejection messages, and
// fuzzfan_compile_test.go type-checks and round-trips the real output.
type FuzzFanTestSuite struct{}

// collectArgs is the discovery half of the pipeline, run over one of the
// shared testdata/sources fixtures.
func collectArgs(t *gotest.T, fixture string) (*packages.Package, []gotestast.FuzzArg) {
	pkg := gotestgen.ExportMustTestPkg(t.T(), fixture)
	c := gotestgen.NewCollector()
	result := c.CollectSuiteSpecs(pkg)
	gotest.Empty(t, result.Errs, "collection errors: %v", result.Errs)
	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)
	return pkg, gotestast.CollectFuzzArgs(pkg, spec.EffectiveTestSuites)
}

func (s *FuzzFanTestSuite) TestCollectFuzzArgs(t *gotest.T) {
	t.It("reads the instantiated type argument of every gotest.Fuzz call", func(it *gotest.T) {
		_, args := collectArgs(it, "TestFuzzCodec_StructTarget")

		gotest.Len(it, args, 2)

		byFunc := map[string]gotestast.FuzzArg{}
		for _, a := range args {
			byFunc[a.FuncName] = a
		}

		create, ok := byFunc["FuzzStructFuzzTestSuite_FuzzCreate"]
		gotest.True(it, ok, "expected an entry for FuzzCreate, got %v", byFunc)
		gotest.Equal(it, "Fuzz", create.Adapter)
		gotest.Equal(it, 0, create.Index)
		gotest.Regexp(it, `\.Request$`, types.TypeString(create.Type, nil))

		native, ok := byFunc["FuzzStructFuzzTestSuite_FuzzNative"]
		gotest.True(it, ok, "expected an entry for FuzzNative, got %v", byFunc)
		gotest.Equal(it, "string", types.TypeString(native.Type, nil))
	})

	t.It("returns nothing for a package with no fuzz methods", func(it *gotest.T) {
		_, args := collectArgs(it, "TestRenderer_FixtureWithChildSuite")
		gotest.Empty(it, args)
	})
}

// buildFans runs the full discovery + emission pipeline over a shared
// testdata/sources fixture.
func buildFans(t *gotest.T, fixture string) (*gotestgen.FuzzFanSet, error) {
	pkg := gotestgen.ExportMustTestPkg(t.T(), fixture)
	c := gotestgen.NewCollector()
	result := c.CollectSuiteSpecs(pkg)
	gotest.Empty(t, result.Errs, "collection errors: %v", result.Errs)
	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)
	return gotestgen.BuildFuzzFans(pkg, spec.EffectiveTestSuites)
}

// build is buildFans for tests that only care about the successful-build
// shape — it fails the test immediately on any generation error.
func (s *FuzzFanTestSuite) build(t *gotest.T, fixture string) *gotestgen.FuzzFanSet {
	set, err := buildFans(t, fixture)
	gotest.NoError(t, err)
	return set
}

func (s *FuzzFanTestSuite) TestStructTarget(t *gotest.T) {
	t.It("emits exactly one fan for the struct target and none for the pass-through one", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Len(it, set.Fans, 1)
		gotest.Equal(it, "gotestfuzz.Fan[Request]{Register: ƒ_fuzzreg_v1_Request, Explode: ƒ_fuzzout_v1_Request, Literal: ƒ_fuzzlits_v1_Request}", set.Fans[0].Expr)
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
	})

	t.It("registers through a direct (*testing.F).Fuzz call with the fanned signature", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Contains(it, set.Source, "func ƒ_fuzzreg_v1_Request(ƒf *testing.F, ƒrun func(*testing.T, Request)) {")
		gotest.Contains(it, set.Source, "ƒf.Fuzz(func(ƒt *testing.T, ƒ0 string, ƒ1 []byte, ƒ2 []byte, ƒ3 []byte, ƒ4 bool, ƒ5 string, ƒ6 []byte) {")
		gotest.Contains(it, set.Source, "ƒrun(ƒt, ƒ_fuzzin_v1_Request(ƒ0, ƒ1, ƒ2, ƒ3, ƒ4, ƒ5, ƒ6))")
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
		gotest.Contains(it, set.Source, "ƒrun(ƒt, ƒ0, ƒ_fuzzin_v1_uint16(ƒ1), ƒ2)")
		set = s.build(it, "TestFuzzCodec_StructTarget")
		gotest.NotContains(it, set.Source, "LeafBytes(ƒ0)", "no []byte field in Request")
	})

	t.It("reports every target's corpus shape, pass-through targets included", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Equal(it, []string{"string", "[]byte", "[]byte", "[]byte", "bool", "string", "[]byte"}, set.ParamsByFunc["FuzzStructFuzzTestSuite_FuzzCreate"])
		gotest.Equal(it, []string{"string"}, set.ParamsByFunc["FuzzStructFuzzTestSuite_FuzzNative"])
	})

	t.It("reports no import beyond gotestfuzz for same-package types", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_StructTarget")
		gotest.Empty(it, set.PkgPaths)
	})

	t.It("emits no fan and no source when every target is pass-through, but still the shapes", func(it *gotest.T) {
		set := s.build(it, "TestCollector_FuzzMethod")
		gotest.NotNil(it, set)
		gotest.Empty(it, set.Fans)
		gotest.Empty(it, set.Source)
		gotest.NotEmpty(it, set.ParamsByFunc)
	})

	t.It("returns nothing at all for a package without fuzz targets", func(it *gotest.T) {
		set := s.build(it, "TestRenderer_FixtureWithChildSuite")
		gotest.Nil(it, set)
	})
}

func (s *FuzzFanTestSuite) TestDeclaredKinds(t *gotest.T) {
	t.It("fans a bare declared int so it rides as a []byte leaf", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_DeclaredKinds")
		gotest.Contains(it, set.Source, "func ƒ_fuzzreg_v1_int(ƒf *testing.F, ƒrun func(*testing.T, int)) {\n\tƒf.Fuzz(func(ƒt *testing.T, ƒ0 []byte) {\n\t\tƒrun(ƒt, ƒ_fuzzin_v1_int(ƒ0))")
		gotest.Equal(it, []string{"[]byte"}, set.ParamsByFunc["FuzzDeclaredFuzzTestSuite_FuzzInt"])
	})

	t.It("emits no fan for an all-pass-through Fuzz2, keeping the native path", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_DeclaredKinds")
		gotest.NotContains(it, set.Source, "Fan2[string, string]")
		gotest.Equal(it, []string{"string", "string"}, set.ParamsByFunc["FuzzDeclaredFuzzTestSuite_FuzzTwoStrings"])
	})

	t.It("fans a mixed Fuzz3 position by position, with a tuple explode and literal", func(it *gotest.T) {
		set := s.build(it, "TestFuzzFan_DeclaredKinds")
		gotest.Contains(it, set.Source, "func ƒ_fuzzreg_v1_string_uint16_slice_byte(ƒf *testing.F, ƒrun func(*testing.T, string, uint16, []byte)) {")
		gotest.Contains(it, set.Source, "ƒf.Fuzz(func(ƒt *testing.T, ƒ0 string, ƒ1 []byte, ƒ2 []byte) {")
		gotest.Contains(it, set.Source, "func ƒ_fuzzexp_v1_string_uint16_slice_byte(ƒa0 string, ƒa1 uint16, ƒa2 []byte) []any {")
		gotest.Contains(it, set.Source, "ƒo = append(ƒo, []any{ƒa0}...)")
		gotest.Contains(it, set.Source, "ƒo = append(ƒo, ƒ_fuzzout_v1_uint16(ƒa1)...)")
		gotest.Contains(it, set.Source, "func ƒ_fuzzlits_v1_string_uint16_slice_byte(ƒa0 string, ƒa1 uint16, ƒa2 []byte) string {")
		gotest.Contains(it, set.Source, `strconv.Quote(string(ƒa0)) + ", " + "uint16(" + strconv.FormatUint(uint64(ƒa1), 10) + ")" + ", " + ƒ_fuzzlit_v1_slice_byte(ƒa2)`)
		gotest.Equal(it, []string{"string", "[]byte", "[]byte"}, set.ParamsByFunc["FuzzDeclaredFuzzTestSuite_FuzzMixed3"])
		var mixed string
		for _, f := range set.Fans {
			if strings.HasPrefix(f.Expr, "gotestfuzz.Fan3[") {
				mixed = f.Expr
			}
		}
		gotest.Equal(it, "gotestfuzz.Fan3[string, uint16, []byte]{Register: ƒ_fuzzreg_v1_string_uint16_slice_byte, Explode: ƒ_fuzzexp_v1_string_uint16_slice_byte, Literal: ƒ_fuzzlits_v1_string_uint16_slice_byte}", mixed)
	})

	t.It("accepts a struct in a multi-argument adapter", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_MultiArgStruct")
		gotest.Len(it, set.Fans, 1)
		gotest.Contains(it, set.Fans[0].Expr, "gotestfuzz.Fan2[Pair, int]{")
		gotest.Contains(it, set.Source, "ƒf.Fuzz(func(ƒt *testing.T, ƒ0 []byte, ƒ1 []byte, ƒ2 []byte) {\n\t\tƒrun(ƒt, ƒ_fuzzin_v1_Pair(ƒ0, ƒ1), ƒ_fuzzin_v1_int(ƒ2))")
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
		gotest.Len(it, set.Fans, 1)
		gotest.Contains(it, set.Fans[0].Expr, "gotestfuzz.Fan[Envelope]{", "the fuzzed type is local, so it stays unqualified")
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
		gotest.Len(it, set.Fans, 1)
		gotest.Contains(it, set.Fans[0].Expr, "Literal: ƒ_fuzzlits_v1_crossdep_ID}")
		gotest.Contains(it, set.Source, `return "crossdep.ID(" + strconv.Quote(string(ƒa0)) + ")"`)
	})
}

// TestAliasDeduplication pins that an alias and its target share one fan.
// They are the same type, so Fan[Alias] and Fan[Inner] are the same
// instantiation — emitting both would put two interchangeable fans on every
// F.
func (s *FuzzFanTestSuite) TestAliasDeduplication(t *gotest.T) {
	t.It("emits one fan and one fan-in for an alias and its target", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_AliasTarget")
		gotest.Len(it, set.Fans, 1, "AliasOf and Inner are the same type")
		gotest.Contains(it, set.Fans[0].Expr, "gotestfuzz.Fan[Inner]{")
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
		gotest.Contains(it, set.Fans[0].Expr, "Literal:", "a *int field now has a self-contained literal form")
		gotest.Contains(it, set.Source, `"&[]int{"`,
			"a non-nil *int renders as the addressable slice-index form, since \"&5\" is not valid Go")
		gotest.Contains(it, set.Source, `return "nil"`,
			"a nil *int still renders as the bare nil literal")
	})

	t.It("inlines the literal of a bare named-basic target into the tuple literal", func(it *gotest.T) {
		set := s.build(it, "TestFuzzCodec_NamedBasicTarget")
		gotest.Contains(it, set.Fans[0].Expr, "Literal: ƒ_fuzzlits_v1_Level}")
		gotest.Contains(it, set.Source, "func ƒ_fuzzlits_v1_Level(ƒa0 Level) string {")
		gotest.Contains(it, set.Source, `return "Level(" + strconv.FormatInt(int64(ƒa0), 10) + ")"`)
	})
}

func (s *FuzzFanTestSuite) TestRejections(t *gotest.T) {
	t.It("rejects an unexported field, naming it and the alternative", func(it *gotest.T) {
		_, err := buildFans(it, "TestFuzzCodec_UnexportedField")
		gotest.ErrorContains(it, err, "FuzzUnexportedFuzzTestSuite_FuzzGuarded")
		gotest.ErrorContains(it, err, "Guarded.mu")
		gotest.ErrorContains(it, err, "unexported fields cannot be set")
		gotest.ErrorContains(it, err, "fuzz the constructor's input")
	})

	t.It("rejects a map field, pointing at the slice-of-pairs workaround", func(it *gotest.T) {
		_, err := buildFans(it, "TestFuzzCodec_MapField")
		gotest.ErrorContains(it, err, "WithMap.Headers")
		gotest.ErrorContains(it, err, "slice of key/value pairs")
	})

	t.It("rejects an interface field", func(it *gotest.T) {
		_, err := buildFans(it, "TestFuzzCodec_InterfaceField")
		gotest.ErrorContains(it, err, "WithAny.Payload")
		gotest.ErrorContains(it, err, "no value can be synthesized")
	})

	t.It("rejects a recursive type rather than emitting an unbounded fan", func(it *gotest.T) {
		_, err := buildFans(it, "TestFuzzCodec_RecursiveType")
		gotest.ErrorContains(it, err, "recursive")
	})

	t.It("rejects a target with no fuzzable leaves", func(it *gotest.T) {
		_, err := buildFans(it, "TestFuzzFan_NoLeaves")
		gotest.ErrorContains(it, err, "FuzzNoLeavesFuzzTestSuite_FuzzMarker")
		gotest.ErrorContains(it, err, "no fuzzable leaves")
	})
}
