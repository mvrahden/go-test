package gotestgen_test

import (
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/packages"
)

// FuzzCodecTestSuite covers fuzz-argument discovery and codec emission — the
// failure mode that matters is silently generating wrong code, so these
// tests assert on emitted source and on rejection messages, and
// fuzzcodec_compile_test.go type-checks and round-trips the real output.
type FuzzCodecTestSuite struct{}

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

func (s *FuzzCodecTestSuite) TestCollectFuzzArgs(t *gotest.T) {
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

// buildCodecs runs the full discovery + emission pipeline over a shared
// testdata/sources fixture.
func buildCodecs(t *gotest.T, fixture string) (*gotestgen.FuzzCodecSet, error) {
	pkg := gotestgen.ExportMustTestPkg(t.T(), fixture)
	c := gotestgen.NewCollector()
	result := c.CollectSuiteSpecs(pkg)
	gotest.Empty(t, result.Errs, "collection errors: %v", result.Errs)
	spec, err := c.ApplyTestSuiteSpecs(result)
	gotest.NoError(t, err)
	return gotestgen.BuildFuzzCodecs(pkg, spec.EffectiveTestSuites)
}

func (s *FuzzCodecTestSuite) TestEmitsCodecsForNonNativeTypes(t *gotest.T) {
	t.It("emits exactly one codec for the struct target and none for the native one", func(it *gotest.T) {
		set, err := buildCodecs(it, "TestFuzzCodec_StructTarget")
		gotest.NoError(it, err)
		gotest.Len(it, set.Codecs, 1)
		gotest.Equal(it, "Request", set.Codecs[0].TypeRef)
		gotest.Equal(it, "ƒ_fuzzdec_v1_Request", set.Codecs[0].DecodeFunc)
		gotest.Equal(it, "ƒ_fuzzenc_v1_Request", set.Codecs[0].EncodeFunc)
	})

	t.It("emits total decoders reading fields in declaration order", func(it *gotest.T) {
		set, err := buildCodecs(it, "TestFuzzCodec_StructTarget")
		gotest.NoError(it, err)

		gotest.Contains(it, set.Source, "func ƒ_fuzzdec_v1_Request(ƒb []byte) Request {")
		gotest.Contains(it, set.Source, "func ƒ_fuzzenc_v1_Request(ƒv Request) []byte {")
		gotest.Contains(it, set.Source, "ƒv.Email = ƒr.String()")
		gotest.Contains(it, set.Source, "ƒv.Age = ƒr.Int()")
		gotest.Contains(it, set.Source, "ƒv.Prio = Priority(ƒr.Int())")
		gotest.Less(it,
			strings.Index(set.Source, "ƒv.Email = "),
			strings.Index(set.Source, "ƒv.Age = "),
			"fields must decode in declaration order")
	})

	t.It("emits a helper per composite type and reuses it", func(it *gotest.T) {
		set, err := buildCodecs(it, "TestFuzzCodec_StructTarget")
		gotest.NoError(it, err)

		gotest.Contains(it, set.Source, "func ƒ_fuzzread_v1_Address(ƒr *gotestruntime.FuzzReader) Address {")
		gotest.Contains(it, set.Source, "func ƒ_fuzzread_v1_slice_string(ƒr *gotestruntime.FuzzReader) []string {")
		gotest.Contains(it, set.Source, "func ƒ_fuzzread_v1_ptr_Address(ƒr *gotestruntime.FuzzReader) *Address {")
	})

	t.It("reports no import beyond gotestruntime for same-package types", func(it *gotest.T) {
		set, err := buildCodecs(it, "TestFuzzCodec_StructTarget")
		gotest.NoError(it, err)
		gotest.Empty(it, set.PkgPaths)
	})

	t.It("returns nothing when every fuzz target is natively fuzzable", func(it *gotest.T) {
		set, err := buildCodecs(it, "TestCollector_FuzzMethod")
		gotest.NoError(it, err)
		gotest.Zero(it, set)
	})
}

// TestCrossPackageTypes covers the shape every external (pxtest) fuzz target
// has: the type under fuzz lives in another package, so it must be emitted
// qualified AND its import path reported, or the generated file references a
// package it never imports and no consuming project compiles.
func (s *FuzzCodecTestSuite) TestCrossPackageTypes(t *gotest.T) {
	t.It("reports the import path of every package the emitted source references", func(it *gotest.T) {
		set, err := buildCodecs(it, "TestFuzzCodec_CrossPackage")
		gotest.NoError(it, err)
		gotest.Equal(it, []string{"testpkg/TestFuzzCodec_CrossDep"}, set.PkgPaths)
	})

	t.It("qualifies the foreign type but not the local one", func(it *gotest.T) {
		set, err := buildCodecs(it, "TestFuzzCodec_CrossPackage")
		gotest.NoError(it, err)

		gotest.Len(it, set.Codecs, 1)
		gotest.Equal(it, "Envelope", set.Codecs[0].TypeRef, "the fuzzed type is local, so it stays unqualified")
		gotest.Contains(it, set.Source, "func ƒ_fuzzread_v1_crossdep_Setting(ƒr *gotestruntime.FuzzReader) crossdep.Setting {")
		gotest.Contains(it, set.Source, "ƒv.S = ƒ_fuzzread_v1_crossdep_Setting(ƒr)")
		gotest.Contains(it, set.Source, "func ƒ_fuzzread_v1_ptr_crossdep_Setting(ƒr *gotestruntime.FuzzReader) *crossdep.Setting {")
	})
}

// TestAliasDeduplication pins that an alias and its target share one codec
// and one helper pair. They are the same type, so Codec[Alias] and
// Codec[Inner] are the same instantiation — emitting both would put two
// interchangeable codecs on every F and make seed attribution ambiguous.
func (s *FuzzCodecTestSuite) TestAliasDeduplication(t *gotest.T) {
	t.It("emits one codec and one helper pair for an alias and its target", func(it *gotest.T) {
		set, err := buildCodecs(it, "TestFuzzCodec_AliasTarget")
		gotest.NoError(it, err)

		gotest.Len(it, set.Codecs, 1, "AliasOf and Inner are the same type")
		gotest.Equal(it, "Inner", set.Codecs[0].TypeRef)
		gotest.Equal(it, 1, strings.Count(set.Source, "func ƒ_fuzzread_v1_Inner("))
		gotest.NotContains(it, set.Source, "ƒ_fuzzread_v1_AliasOf")
	})
}

func (s *FuzzCodecTestSuite) TestRejections(t *gotest.T) {
	t.It("rejects an unexported field, naming it and the alternative", func(it *gotest.T) {
		_, err := buildCodecs(it, "TestFuzzCodec_UnexportedField")
		gotest.ErrorContains(it, err, "FuzzUnexportedFuzzTestSuite_FuzzGuarded")
		gotest.ErrorContains(it, err, "Guarded.mu")
		gotest.ErrorContains(it, err, "unexported fields cannot be set")
		gotest.ErrorContains(it, err, "fuzz the constructor's input")
	})

	t.It("rejects a map field, pointing at the slice-of-pairs workaround", func(it *gotest.T) {
		_, err := buildCodecs(it, "TestFuzzCodec_MapField")
		gotest.ErrorContains(it, err, "WithMap.Headers")
		gotest.ErrorContains(it, err, "slice of key/value pairs")
	})

	t.It("rejects an interface field", func(it *gotest.T) {
		_, err := buildCodecs(it, "TestFuzzCodec_InterfaceField")
		gotest.ErrorContains(it, err, "WithAny.Payload")
		gotest.ErrorContains(it, err, "no value can be synthesized")
	})

	t.It("rejects a recursive type rather than emitting an unbounded decoder", func(it *gotest.T) {
		_, err := buildCodecs(it, "TestFuzzCodec_RecursiveType")
		gotest.ErrorContains(it, err, "recursive")
	})

	t.It("rejects a non-native argument to a multi-argument adapter", func(it *gotest.T) {
		_, err := buildCodecs(it, "TestFuzzCodec_MultiArgStruct")
		gotest.ErrorContains(it, err, "gotest.Fuzz2")
		gotest.ErrorContains(it, err, "wrap them in a single struct")
	})
}
