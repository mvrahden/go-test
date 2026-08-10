package scaffold //nolint:stdlib-test

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantPkg  string
		wantType string
		wantErr  bool
	}{
		{
			name:     "simple package and type",
			input:    "./pkg/user.UserService",
			wantPkg:  "./pkg/user",
			wantType: "UserService",
		},
		{
			name:     "versioned package",
			input:    "./pkg/user.v2.UserService",
			wantPkg:  "./pkg/user.v2",
			wantType: "UserService",
		},
		{
			name:     "nested package",
			input:    "./internal/auth/handler.AuthHandler",
			wantPkg:  "./internal/auth/handler",
			wantType: "AuthHandler",
		},
		{
			name:    "no type name",
			input:   "./pkg/user",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "dot but lowercase after (not a type)",
			input:   "./pkg/user.lowercase",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkg, typeName, err := ParseTarget(tc.input)
			if tc.wantErr {
				gotest.Error(t, err, "expected error, got nil")
				return
			}
			gotest.NoError(t, err, "unexpected error: %v", err)
			gotest.Equal(t, tc.wantPkg, pkg)
			gotest.Equal(t, tc.wantType, typeName)
		})
	}
}

func TestIntrospectType_Struct(t *testing.T) {
	// Use the testdata sampletype package
	info, err := IntrospectType("./testdata/sampletype", "UserService")
	gotest.NoError(t, err, "IntrospectType failed: %v", err)

	gotest.Equal(t, "UserService", info.Name)
	gotest.Equal(t, "sampletype", info.PkgName)
	gotest.False(t, info.IsInterface)
	gotest.NotEmpty(t, info.PkgDir)

	// Should have exactly 3 exported methods: Create, Delete, GetByID (sorted)
	gotest.Len(t, info.Methods, 3)

	wantNames := []string{"Create", "Delete", "GetByID"}
	for i, want := range wantNames {
		gotest.Equal(t, want, info.Methods[i].Name)
	}

	// Create returns error
	gotest.True(t, info.Methods[0].ReturnsError, "Create should return error")
	// Delete returns error
	gotest.True(t, info.Methods[1].ReturnsError, "Delete should return error")
	// GetByID returns error
	gotest.True(t, info.Methods[2].ReturnsError, "GetByID should return error")
}

func TestIntrospectType_Interface(t *testing.T) {
	info, err := IntrospectType("./testdata/sampletype", "Validator")
	gotest.NoError(t, err, "IntrospectType failed: %v", err)

	gotest.Equal(t, "Validator", info.Name)
	gotest.True(t, info.IsInterface)

	gotest.Len(t, info.Methods, 2)

	// Sorted: IsValid, Validate
	gotest.Equal(t, "IsValid", info.Methods[0].Name)
	gotest.Equal(t, "Validate", info.Methods[1].Name)

	// IsValid does NOT return error
	gotest.False(t, info.Methods[0].ReturnsError, "IsValid should not return error")
	// Validate returns error
	gotest.True(t, info.Methods[1].ReturnsError, "Validate should return error")
}

func TestGenerateScaffold_Struct(t *testing.T) {
	info := &TypeInfo{
		Name:    "UserService",
		PkgName: "user",
		Methods: []MethodInfo{
			{Name: "Create", Signature: "(ctx context.Context, name string) error", ReturnsError: true},
			{Name: "Delete", Signature: "(ctx context.Context, id string) error", ReturnsError: true},
			{Name: "List", Signature: "() []string", ReturnsError: false},
		},
	}

	out, err := GenerateScaffold(info)
	gotest.NoError(t, err, "GenerateScaffold failed: %v", err)

	src := string(out)

	// Check package declaration
	gotest.Contains(t, src, "package user")

	// Check import
	gotest.Contains(t, src, `"github.com/mvrahden/go-test/pkg/gotest"`)

	// Check suite struct
	gotest.Contains(t, src, "type UserServiceTestSuite struct")
	gotest.Contains(t, src, "sut *UserService")

	// Check BeforeEach
	gotest.Contains(t, src, "func (s *UserServiceTestSuite) BeforeEach(t *gotest.T)")

	// Check test methods for error-returning methods
	gotest.Contains(t, src, "func (s *UserServiceTestSuite) TestCreate(t *gotest.T)")
	gotest.Contains(t, src, `t.It("succeeds"`)
	gotest.Contains(t, src, `t.It("returns error"`)

	// Check test method for non-error method
	gotest.Contains(t, src, "func (s *UserServiceTestSuite) TestList(t *gotest.T)")
	// TestList should have "works" It block
	gotest.Contains(t, src, `t.It("works"`)
}

func TestGenerateContractScaffold_Interface(t *testing.T) {
	info := &TypeInfo{
		Name:        "Validator",
		PkgName:     "validation",
		IsInterface: true,
		Methods: []MethodInfo{
			{Name: "IsValid", Signature: "(input string) bool", ReturnsError: false},
			{Name: "Validate", Signature: "(input string) error", ReturnsError: true},
		},
	}

	out, err := GenerateContractScaffold(info)
	gotest.NoError(t, err, "GenerateContractScaffold failed: %v", err)

	src := string(out)

	// Check package declaration
	gotest.Contains(t, src, "package validation")

	// Check generic suite struct
	gotest.Contains(t, src, "type ValidatorContractTestSuite[T Validator] struct")
	gotest.Contains(t, src, "factory func() T")

	// Check BeforeEach uses factory
	gotest.Contains(t, src, "s.sut = s.factory()")

	// Check test methods
	gotest.Contains(t, src, "func (s *ValidatorContractTestSuite[T]) TestValidate(t *gotest.T)")
	gotest.Contains(t, src, "func (s *ValidatorContractTestSuite[T]) TestIsValid(t *gotest.T)")

	// Check instantiation comment
	gotest.Contains(t, src, "type MyValidatorTestSuite = ValidatorContractTestSuite[*MyImpl]")
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"UserService", "user_service"},
		{"HTTPClient", "http_client"},
		{"ID", "id"},
		{"Simple", "simple"},
		{"getByID", "get_by_id"},
		{"HTMLParser", "html_parser"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := toSnakeCase(tc.input)
			gotest.Equal(t, tc.want, got)
		})
	}
}

func TestIntrospectFile_Funcs(t *testing.T) {
	info, err := IntrospectFile("./testdata/sampletype", "funcs.go")
	gotest.NoError(t, err, "IntrospectFile failed: %v", err)

	gotest.Equal(t, "FuncsTestSuite", info.SuiteName)
	gotest.Equal(t, "sampletype", info.PkgName)
	gotest.NotEmpty(t, info.PkgDir)
	gotest.Len(t, info.Funcs, 2)
	wantNames := []string{"ApplyTax", "CalculateDiscount"}
	for i, want := range wantNames {
		gotest.Equal(t, want, info.Funcs[i].Name)
	}
}

func TestIntrospectFile_NoExported(t *testing.T) {
	info, err := IntrospectFile("./testdata/sampletype", "types.go")
	gotest.NoError(t, err, "IntrospectFile failed: %v", err)
	gotest.Len(t, info.Funcs, 1)
	gotest.Equal(t, "NewUserService", info.Funcs[0].Name)
}

func TestGenerateFileScaffold(t *testing.T) {
	info := &FileInfo{
		SuiteName: "CalcTestSuite",
		PkgName:   "pricing",
		Funcs: []FuncInfo{
			{Name: "ApplyTax", Signature: "(amount float64, region string) float64"},
			{Name: "CalculateDiscount", Signature: "(amount float64, tier string) float64"},
		},
	}

	out, err := GenerateFileScaffold(info)
	gotest.NoError(t, err, "GenerateFileScaffold failed: %v", err)

	src := string(out)

	gotest.Contains(t, src, "package pricing")
	gotest.Contains(t, src, `"github.com/mvrahden/go-test/pkg/gotest"`)
	gotest.Contains(t, src, "type CalcTestSuite struct")
	gotest.NotContains(t, src, "gotest.TestSuite", "scaffold must not embed gotest.TestSuite — no such type exists")
	_, perr := parser.ParseFile(token.NewFileSet(), "scaffold.go", src, 0)
	gotest.NoError(t, perr, "generated scaffold does not parse")
	gotest.NotContains(t, src, "sut", "file-scoped scaffold should NOT have sut field")
	gotest.NotContains(t, src, "BeforeEach", "file-scoped scaffold should NOT have BeforeEach")
	gotest.Contains(t, src, "func (s *CalcTestSuite) TestApplyTax(t *gotest.T)")
	gotest.Contains(t, src, "func (s *CalcTestSuite) TestCalculateDiscount(t *gotest.T)")
}

func TestScaffoldIntegration_File(t *testing.T) {
	info, err := IntrospectFile("./testdata/sampletype", "funcs.go")
	gotest.NoError(t, err, "IntrospectFile failed: %v", err)

	out, err := GenerateFileScaffold(info)
	gotest.NoError(t, err, "GenerateFileScaffold failed: %v", err)

	gotest.MatchSnapshot(t, string(out))
}

func TestScaffoldIntegration(t *testing.T) {
	info, err := IntrospectType("./testdata/sampletype", "UserService")
	gotest.NoError(t, err, "IntrospectType failed: %v", err)

	out, err := GenerateScaffold(info)
	gotest.NoError(t, err, "GenerateScaffold failed: %v", err)

	gotest.MatchSnapshot(t, string(out))
}

func TestScaffoldIntegration_Interface(t *testing.T) {
	info, err := IntrospectType("./testdata/sampletype", "Validator")
	gotest.NoError(t, err, "IntrospectType failed: %v", err)

	out, err := GenerateContractScaffold(info)
	gotest.NoError(t, err, "GenerateContractScaffold failed: %v", err)

	gotest.MatchSnapshot(t, string(out))
}
