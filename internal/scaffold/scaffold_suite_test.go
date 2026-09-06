package scaffold_test

import (
	"go/parser"
	"go/token"

	"github.com/mvrahden/go-test/internal/scaffold"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScaffoldTestSuite covers target parsing, type and file introspection, and
// the generated suite skeletons, whose full text is pinned by snapshots.
type ScaffoldTestSuite struct{}

func (s *ScaffoldTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type scaffoldCtx struct{}

func (s *ScaffoldTestSuite) BeforeEach(t *gotest.T) *scaffoldCtx { return &scaffoldCtx{} }

func (s *ScaffoldTestSuite) TestParseTarget(t *gotest.T, _ *scaffoldCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc     string
		input    string
		wantPkg  string
		wantType string
		wantErr  bool
	}{
		{Desc: "simple package and type", input: "./pkg/user.UserService", wantPkg: "./pkg/user", wantType: "UserService"},
		{Desc: "versioned package", input: "./pkg/user.v2.UserService", wantPkg: "./pkg/user.v2", wantType: "UserService"},
		{Desc: "nested package", input: "./internal/auth/handler.AuthHandler", wantPkg: "./internal/auth/handler", wantType: "AuthHandler"},
		{Desc: "no type name", input: "./pkg/user", wantErr: true},
		{Desc: "empty string", input: "", wantErr: true},
		{Desc: "dot but lowercase after (not a type)", input: "./pkg/user.lowercase", wantErr: true},
	}) {
		pkg, typeName, err := scaffold.ParseTarget(tc.input)
		if tc.wantErr {
			gotest.Error(sub, err, "expected error, got nil")
			continue
		}
		gotest.NoError(sub, err, "unexpected error: %v", err)
		gotest.Equal(sub, tc.wantPkg, pkg)
		gotest.Equal(sub, tc.wantType, typeName)
	}
}

func (s *ScaffoldTestSuite) TestIntrospectType_Struct(t *gotest.T, _ *scaffoldCtx) {
	info, err := scaffold.IntrospectType("./testdata/sampletype", "UserService")
	gotest.NoError(t, err, "IntrospectType failed: %v", err)

	gotest.Equal(t, "UserService", info.Name)
	gotest.Equal(t, "sampletype", info.PkgName)
	gotest.False(t, info.IsInterface)
	gotest.NotEmpty(t, info.PkgDir)

	// Exactly the three exported methods, sorted.
	gotest.Len(t, info.Methods, 3)
	for i, want := range []string{"Create", "Delete", "GetByID"} {
		gotest.Equal(t, want, info.Methods[i].Name)
		gotest.True(t, info.Methods[i].ReturnsError, "%s should return error", want)
	}
}

func (s *ScaffoldTestSuite) TestIntrospectType_Interface(t *gotest.T, _ *scaffoldCtx) {
	info, err := scaffold.IntrospectType("./testdata/sampletype", "Validator")
	gotest.NoError(t, err, "IntrospectType failed: %v", err)

	gotest.Equal(t, "Validator", info.Name)
	gotest.True(t, info.IsInterface)
	gotest.Len(t, info.Methods, 2)

	// Sorted: IsValid, Validate; only Validate returns error.
	gotest.Equal(t, "IsValid", info.Methods[0].Name)
	gotest.Equal(t, "Validate", info.Methods[1].Name)
	gotest.False(t, info.Methods[0].ReturnsError, "IsValid should not return error")
	gotest.True(t, info.Methods[1].ReturnsError, "Validate should return error")
}

func (s *ScaffoldTestSuite) TestGenerateScaffold_Struct(t *gotest.T, _ *scaffoldCtx) {
	info := &scaffold.TypeInfo{
		Name:    "UserService",
		PkgName: "user",
		Methods: []scaffold.MethodInfo{
			{Name: "Create", Signature: "(ctx context.Context, name string) error", ReturnsError: true},
			{Name: "Delete", Signature: "(ctx context.Context, id string) error", ReturnsError: true},
			{Name: "List", Signature: "() []string", ReturnsError: false},
		},
	}

	out, err := scaffold.GenerateScaffold(info)
	gotest.NoError(t, err, "GenerateScaffold failed: %v", err)
	src := string(out)

	gotest.Contains(t, src, "package user")
	gotest.Contains(t, src, `"github.com/mvrahden/go-test/pkg/gotest"`)
	gotest.Contains(t, src, "type UserServiceTestSuite struct")
	gotest.Contains(t, src, "sut *UserService")
	gotest.Contains(t, src, "func (s *UserServiceTestSuite) BeforeEach(t *gotest.T)")
	// Error-returning methods get a success and an error block.
	gotest.Contains(t, src, "func (s *UserServiceTestSuite) TestCreate(t *gotest.T)")
	gotest.Contains(t, src, `t.It("succeeds"`)
	gotest.Contains(t, src, `t.It("returns error"`)
	// A plain method gets a "works" block.
	gotest.Contains(t, src, "func (s *UserServiceTestSuite) TestList(t *gotest.T)")
	gotest.Contains(t, src, `t.It("works"`)
}

func (s *ScaffoldTestSuite) TestGenerateContractScaffold_Interface(t *gotest.T, _ *scaffoldCtx) {
	info := &scaffold.TypeInfo{
		Name:        "Validator",
		PkgName:     "validation",
		IsInterface: true,
		Methods: []scaffold.MethodInfo{
			{Name: "IsValid", Signature: "(input string) bool", ReturnsError: false},
			{Name: "Validate", Signature: "(input string) error", ReturnsError: true},
		},
	}

	out, err := scaffold.GenerateContractScaffold(info)
	gotest.NoError(t, err, "GenerateContractScaffold failed: %v", err)
	src := string(out)

	gotest.Contains(t, src, "package validation")
	gotest.Contains(t, src, "type ValidatorContractTestSuite[T Validator] struct")
	gotest.Contains(t, src, "factory func() T")
	gotest.Contains(t, src, "s.sut = s.factory()")
	gotest.Contains(t, src, "func (s *ValidatorContractTestSuite[T]) TestValidate(t *gotest.T)")
	gotest.Contains(t, src, "func (s *ValidatorContractTestSuite[T]) TestIsValid(t *gotest.T)")
	gotest.Contains(t, src, "type MyValidatorTestSuite = ValidatorContractTestSuite[*MyImpl]")
}

func (s *ScaffoldTestSuite) TestToSnakeCase(t *gotest.T, _ *scaffoldCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		want string
	}{
		{Desc: "UserService", want: "user_service"},
		{Desc: "HTTPClient", want: "http_client"},
		{Desc: "ID", want: "id"},
		{Desc: "Simple", want: "simple"},
		{Desc: "getByID", want: "get_by_id"},
		{Desc: "HTMLParser", want: "html_parser"},
	}) {
		gotest.Equal(sub, tc.want, scaffold.ExportToSnakeCase(tc.Desc))
	}
}

func (s *ScaffoldTestSuite) TestIntrospectFile_Funcs(t *gotest.T, _ *scaffoldCtx) {
	info, err := scaffold.IntrospectFile("./testdata/sampletype", "funcs.go")
	gotest.NoError(t, err, "IntrospectFile failed: %v", err)

	gotest.Equal(t, "FuncsTestSuite", info.SuiteName)
	gotest.Equal(t, "sampletype", info.PkgName)
	gotest.NotEmpty(t, info.PkgDir)
	gotest.Len(t, info.Funcs, 2)
	for i, want := range []string{"ApplyTax", "CalculateDiscount"} {
		gotest.Equal(t, want, info.Funcs[i].Name)
	}
}

func (s *ScaffoldTestSuite) TestIntrospectFile_NoExported(t *gotest.T, _ *scaffoldCtx) {
	info, err := scaffold.IntrospectFile("./testdata/sampletype", "types.go")
	gotest.NoError(t, err, "IntrospectFile failed: %v", err)
	gotest.Len(t, info.Funcs, 1)
	gotest.Equal(t, "NewUserService", info.Funcs[0].Name)
}

func (s *ScaffoldTestSuite) TestGenerateFileScaffold(t *gotest.T, _ *scaffoldCtx) {
	info := &scaffold.FileInfo{
		SuiteName: "CalcTestSuite",
		PkgName:   "pricing",
		Funcs: []scaffold.FuncInfo{
			{Name: "ApplyTax", Signature: "(amount float64, region string) float64"},
			{Name: "CalculateDiscount", Signature: "(amount float64, tier string) float64"},
		},
	}

	out, err := scaffold.GenerateFileScaffold(info)
	gotest.NoError(t, err, "GenerateFileScaffold failed: %v", err)
	src := string(out)

	gotest.Contains(t, src, "package pricing")
	gotest.Contains(t, src, `"github.com/mvrahden/go-test/pkg/gotest"`)
	gotest.Contains(t, src, "type CalcTestSuite struct")
	gotest.NotContains(t, src, "gotest.TestSuite", "scaffold must not embed gotest.TestSuite — no such type exists")
	_, perr := parser.ParseFile(token.NewFileSet(), "scaffold.go", src, 0)
	gotest.NoError(t, perr, "generated scaffold does not parse")
	// File-scoped scaffolds have no subject and no BeforeEach.
	gotest.NotContains(t, src, "sut")
	gotest.NotContains(t, src, "BeforeEach")
	gotest.Contains(t, src, "func (s *CalcTestSuite) TestApplyTax(t *gotest.T)")
	gotest.Contains(t, src, "func (s *CalcTestSuite) TestCalculateDiscount(t *gotest.T)")
}

func (s *ScaffoldTestSuite) TestScaffoldIntegration_File(t *gotest.T, _ *scaffoldCtx) {
	info, err := scaffold.IntrospectFile("./testdata/sampletype", "funcs.go")
	gotest.NoError(t, err, "IntrospectFile failed: %v", err)

	out, err := scaffold.GenerateFileScaffold(info)
	gotest.NoError(t, err, "GenerateFileScaffold failed: %v", err)

	gotest.MatchSnapshot(t, string(out))
}

func (s *ScaffoldTestSuite) TestScaffoldIntegration(t *gotest.T, _ *scaffoldCtx) {
	info, err := scaffold.IntrospectType("./testdata/sampletype", "UserService")
	gotest.NoError(t, err, "IntrospectType failed: %v", err)

	out, err := scaffold.GenerateScaffold(info)
	gotest.NoError(t, err, "GenerateScaffold failed: %v", err)

	gotest.MatchSnapshot(t, string(out))
}

func (s *ScaffoldTestSuite) TestScaffoldIntegration_Interface(t *gotest.T, _ *scaffoldCtx) {
	info, err := scaffold.IntrospectType("./testdata/sampletype", "Validator")
	gotest.NoError(t, err, "IntrospectType failed: %v", err)

	out, err := scaffold.GenerateContractScaffold(info)
	gotest.NoError(t, err, "GenerateContractScaffold failed: %v", err)

	gotest.MatchSnapshot(t, string(out))
}
