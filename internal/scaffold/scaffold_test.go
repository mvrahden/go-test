package scaffold //nolint:stdlib-test

import (
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantPkg    string
		wantType   string
		wantErr    bool
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
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pkg != tc.wantPkg {
				t.Errorf("pkg: want %q, got %q", tc.wantPkg, pkg)
			}
			if typeName != tc.wantType {
				t.Errorf("typeName: want %q, got %q", tc.wantType, typeName)
			}
		})
	}
}

func TestIntrospectType_Struct(t *testing.T) {
	// Use the testdata sampletype package
	info, err := IntrospectType("./testdata/sampletype", "UserService")
	if err != nil {
		t.Fatalf("IntrospectType failed: %v", err)
	}

	if info.Name != "UserService" {
		t.Errorf("Name: want %q, got %q", "UserService", info.Name)
	}
	if info.PkgName != "sampletype" {
		t.Errorf("PkgName: want %q, got %q", "sampletype", info.PkgName)
	}
	if info.IsInterface {
		t.Error("expected IsInterface=false for struct")
	}
	if info.PkgDir == "" {
		t.Error("PkgDir should not be empty")
	}

	// Should have exactly 3 exported methods: Create, Delete, GetByID (sorted)
	if len(info.Methods) != 3 {
		t.Fatalf("expected 3 methods, got %d: %+v", len(info.Methods), info.Methods)
	}

	wantNames := []string{"Create", "Delete", "GetByID"}
	for i, want := range wantNames {
		if info.Methods[i].Name != want {
			t.Errorf("method[%d]: want %q, got %q", i, want, info.Methods[i].Name)
		}
	}

	// Create returns error
	if !info.Methods[0].ReturnsError {
		t.Error("Create should return error")
	}
	// Delete returns error
	if !info.Methods[1].ReturnsError {
		t.Error("Delete should return error")
	}
	// GetByID returns error
	if !info.Methods[2].ReturnsError {
		t.Error("GetByID should return error")
	}
}

func TestIntrospectType_Interface(t *testing.T) {
	info, err := IntrospectType("./testdata/sampletype", "Validator")
	if err != nil {
		t.Fatalf("IntrospectType failed: %v", err)
	}

	if info.Name != "Validator" {
		t.Errorf("Name: want %q, got %q", "Validator", info.Name)
	}
	if !info.IsInterface {
		t.Error("expected IsInterface=true for interface")
	}

	if len(info.Methods) != 2 {
		t.Fatalf("expected 2 methods, got %d: %+v", len(info.Methods), info.Methods)
	}

	// Sorted: IsValid, Validate
	if info.Methods[0].Name != "IsValid" {
		t.Errorf("method[0]: want %q, got %q", "IsValid", info.Methods[0].Name)
	}
	if info.Methods[1].Name != "Validate" {
		t.Errorf("method[1]: want %q, got %q", "Validate", info.Methods[1].Name)
	}

	// IsValid does NOT return error
	if info.Methods[0].ReturnsError {
		t.Error("IsValid should not return error")
	}
	// Validate returns error
	if !info.Methods[1].ReturnsError {
		t.Error("Validate should return error")
	}
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
	if err != nil {
		t.Fatalf("GenerateScaffold failed: %v", err)
	}

	src := string(out)

	// Check package declaration
	if !strings.Contains(src, "package user") {
		t.Error("missing package declaration")
	}

	// Check import
	if !strings.Contains(src, `"github.com/mvrahden/go-test/pkg/gotest"`) {
		t.Error("missing gotest import")
	}

	// Check suite struct
	if !strings.Contains(src, "type UserServiceTestSuite struct") {
		t.Error("missing test suite struct")
	}
	if !strings.Contains(src, "sut *UserService") {
		t.Error("missing sut field")
	}

	// Check BeforeEach
	if !strings.Contains(src, "func (s *UserServiceTestSuite) BeforeEach(t *gotest.T)") {
		t.Error("missing BeforeEach method")
	}

	// Check test methods for error-returning methods
	if !strings.Contains(src, "func (s *UserServiceTestSuite) TestCreate(t *gotest.T)") {
		t.Error("missing TestCreate method")
	}
	if !strings.Contains(src, `t.It("succeeds"`) {
		t.Error("missing 'succeeds' It block for error-returning method")
	}
	if !strings.Contains(src, `t.It("returns error"`) {
		t.Error("missing 'returns error' It block for error-returning method")
	}

	// Check test method for non-error method
	if !strings.Contains(src, "func (s *UserServiceTestSuite) TestList(t *gotest.T)") {
		t.Error("missing TestList method")
	}
	// TestList should have "works" It block
	if !strings.Contains(src, `t.It("works"`) {
		t.Error("missing 'works' It block for non-error method")
	}
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
	if err != nil {
		t.Fatalf("GenerateContractScaffold failed: %v", err)
	}

	src := string(out)

	// Check package declaration
	if !strings.Contains(src, "package validation") {
		t.Error("missing package declaration")
	}

	// Check generic suite struct
	if !strings.Contains(src, "type ValidatorContractTestSuite[T Validator] struct") {
		t.Error("missing generic contract test suite struct")
	}
	if !strings.Contains(src, "factory func() T") {
		t.Error("missing factory field")
	}

	// Check BeforeEach uses factory
	if !strings.Contains(src, "s.sut = s.factory()") {
		t.Error("missing factory call in BeforeEach")
	}

	// Check test methods
	if !strings.Contains(src, "func (s *ValidatorContractTestSuite[T]) TestValidate(t *gotest.T)") {
		t.Error("missing TestValidate method")
	}
	if !strings.Contains(src, "func (s *ValidatorContractTestSuite[T]) TestIsValid(t *gotest.T)") {
		t.Error("missing TestIsValid method")
	}

	// Check instantiation comment
	if !strings.Contains(src, "type MyValidatorTestSuite = ValidatorContractTestSuite[*MyImpl]") {
		t.Error("missing instantiation comment")
	}
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
			if got != tc.want {
				t.Errorf("toSnakeCase(%q): want %q, got %q", tc.input, tc.want, got)
			}
		})
	}
}

func TestIntrospectFile_Funcs(t *testing.T) {
	info, err := IntrospectFile("./testdata/sampletype", "funcs.go")
	if err != nil {
		t.Fatalf("IntrospectFile failed: %v", err)
	}

	if info.SuiteName != "FuncsTestSuite" {
		t.Errorf("SuiteName: want %q, got %q", "FuncsTestSuite", info.SuiteName)
	}
	if info.PkgName != "sampletype" {
		t.Errorf("PkgName: want %q, got %q", "sampletype", info.PkgName)
	}
	if info.PkgDir == "" {
		t.Error("PkgDir should not be empty")
	}
	if len(info.Funcs) != 2 {
		t.Fatalf("expected 2 funcs, got %d: %+v", len(info.Funcs), info.Funcs)
	}
	wantNames := []string{"ApplyTax", "CalculateDiscount"}
	for i, want := range wantNames {
		if info.Funcs[i].Name != want {
			t.Errorf("func[%d]: want %q, got %q", i, want, info.Funcs[i].Name)
		}
	}
}

func TestIntrospectFile_NoExported(t *testing.T) {
	info, err := IntrospectFile("./testdata/sampletype", "types.go")
	if err != nil {
		t.Fatalf("IntrospectFile failed: %v", err)
	}
	if len(info.Funcs) != 1 {
		t.Fatalf("expected 1 exported func (NewUserService), got %d: %+v", len(info.Funcs), info.Funcs)
	}
	if info.Funcs[0].Name != "NewUserService" {
		t.Errorf("func[0]: want %q, got %q", "NewUserService", info.Funcs[0].Name)
	}
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
	if err != nil {
		t.Fatalf("GenerateFileScaffold failed: %v", err)
	}

	src := string(out)

	if !strings.Contains(src, "package pricing") {
		t.Error("missing package declaration")
	}
	if !strings.Contains(src, `"github.com/mvrahden/go-test/pkg/gotest"`) {
		t.Error("missing gotest import")
	}
	if !strings.Contains(src, "type CalcTestSuite struct") {
		t.Error("missing test suite struct")
	}
	if !strings.Contains(src, "gotest.TestSuite") {
		t.Error("missing embedded TestSuite")
	}
	if strings.Contains(src, "sut") {
		t.Error("file-scoped scaffold should NOT have sut field")
	}
	if strings.Contains(src, "BeforeEach") {
		t.Error("file-scoped scaffold should NOT have BeforeEach")
	}
	if !strings.Contains(src, "func (s *CalcTestSuite) TestApplyTax(t *gotest.T)") {
		t.Error("missing TestApplyTax method")
	}
	if !strings.Contains(src, "func (s *CalcTestSuite) TestCalculateDiscount(t *gotest.T)") {
		t.Error("missing TestCalculateDiscount method")
	}
}

func TestScaffoldIntegration_File(t *testing.T) {
	info, err := IntrospectFile("./testdata/sampletype", "funcs.go")
	if err != nil {
		t.Fatalf("IntrospectFile failed: %v", err)
	}

	out, err := GenerateFileScaffold(info)
	if err != nil {
		t.Fatalf("GenerateFileScaffold failed: %v", err)
	}

	gotest.MatchSnapshot(t, string(out))
}

func TestScaffoldIntegration(t *testing.T) {
	info, err := IntrospectType("./testdata/sampletype", "UserService")
	if err != nil {
		t.Fatalf("IntrospectType failed: %v", err)
	}

	out, err := GenerateScaffold(info)
	if err != nil {
		t.Fatalf("GenerateScaffold failed: %v", err)
	}

	gotest.MatchSnapshot(t, string(out))
}

func TestScaffoldIntegration_Interface(t *testing.T) {
	info, err := IntrospectType("./testdata/sampletype", "Validator")
	if err != nil {
		t.Fatalf("IntrospectType failed: %v", err)
	}

	out, err := GenerateContractScaffold(info)
	if err != nil {
		t.Fatalf("GenerateContractScaffold failed: %v", err)
	}

	gotest.MatchSnapshot(t, string(out))
}
