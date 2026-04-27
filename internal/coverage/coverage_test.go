package coverage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/cover"
)

func TestFindMethodPositions(t *testing.T) {
	dir := t.TempDir()
	src := `package foo

type UserService struct{}

func (s *UserService) Create() {}
func (s *UserService) Delete() {}
func (s UserService) Get() {}
func NotAMethod() {}
`
	file := filepath.Join(dir, "user.go")
	if err := os.WriteFile(file, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	positions := findMethodPositions([]string{file})

	if _, ok := positions["UserService.Create"]; !ok {
		t.Error("expected UserService.Create")
	}
	if _, ok := positions["UserService.Delete"]; !ok {
		t.Error("expected UserService.Delete")
	}
	if _, ok := positions["UserService.Get"]; !ok {
		t.Error("expected UserService.Get (value receiver)")
	}
	if _, ok := positions["NotAMethod"]; ok {
		t.Error("top-level function should not appear")
	}
}

func TestFindMethodPositionsSkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	src := `package foo

type Helper struct{}

func (h *Helper) DoStuff() {}
`
	file := filepath.Join(dir, "helper_test.go")
	if err := os.WriteFile(file, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	positions := findMethodPositions([]string{file})
	if len(positions) != 0 {
		t.Errorf("expected 0 positions from test file, got %d", len(positions))
	}
}

func TestFindMethodPositionsLineRanges(t *testing.T) {
	dir := t.TempDir()
	src := `package foo

type Svc struct{}

func (s *Svc) Short() { return }

func (s *Svc) Long() {
	_ = 1
	_ = 2
	_ = 3
}
`
	file := filepath.Join(dir, "svc.go")
	if err := os.WriteFile(file, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	positions := findMethodPositions([]string{file})

	short := positions["Svc.Short"]
	if short.startLine != short.endLine {
		t.Errorf("Short: expected single-line body, got %d-%d", short.startLine, short.endLine)
	}

	long := positions["Svc.Long"]
	if long.endLine-long.startLine < 3 {
		t.Errorf("Long: expected multi-line body, got %d-%d", long.startLine, long.endLine)
	}
}

func TestCoverageBlockMatching(t *testing.T) {
	blocks := []cover.ProfileBlock{
		{StartLine: 5, StartCol: 1, EndLine: 7, EndCol: 1, NumStmt: 1, Count: 1},
		{StartLine: 10, StartCol: 1, EndLine: 15, EndCol: 1, NumStmt: 3, Count: 0},
	}

	t.Run("covered block overlaps method", func(t *testing.T) {
		pos := methodPos{startLine: 5, endLine: 7}
		covered := isBlockCovered(blocks, pos)
		if !covered {
			t.Error("expected covered")
		}
	})

	t.Run("uncovered block does not match", func(t *testing.T) {
		pos := methodPos{startLine: 10, endLine: 15}
		covered := isBlockCovered(blocks, pos)
		if covered {
			t.Error("expected uncovered")
		}
	})

	t.Run("partial overlap counts as covered", func(t *testing.T) {
		pos := methodPos{startLine: 6, endLine: 9}
		covered := isBlockCovered(blocks, pos)
		if !covered {
			t.Error("expected covered (partial overlap)")
		}
	})

	t.Run("no overlap means uncovered", func(t *testing.T) {
		pos := methodPos{startLine: 20, endLine: 25}
		covered := isBlockCovered(blocks, pos)
		if covered {
			t.Error("expected uncovered (no overlap)")
		}
	})
}

func TestRender(t *testing.T) {
	report := &Report{
		Packages: []PackageReport{
			{
				Path: "example/user",
				Types: []TypeReport{
					{
						Name: "UserService",
						Methods: []MethodReport{
							{Name: "Create", Covered: true},
							{Name: "Delete", Covered: false},
							{Name: "Get", Covered: true},
						},
					},
				},
			},
		},
		Total:   3,
		Covered: 2,
	}

	var buf bytes.Buffer
	Render(&buf, report)
	out := buf.String()

	if !strings.Contains(out, "UserService: 2/3 methods covered (66%)") {
		t.Errorf("expected type summary, got:\n%s", out)
	}
	if !strings.Contains(out, "✓ Create") {
		t.Errorf("expected covered method Create, got:\n%s", out)
	}
	if !strings.Contains(out, "✗ Delete") {
		t.Errorf("expected uncovered method Delete, got:\n%s", out)
	}
	if !strings.Contains(out, "Overall: 2/3 methods covered (66%)") {
		t.Errorf("expected overall summary, got:\n%s", out)
	}
}

func TestRenderMultiPackage(t *testing.T) {
	report := &Report{
		Packages: []PackageReport{
			{Path: "pkg/a", Types: []TypeReport{{Name: "A", Methods: []MethodReport{{Name: "Do", Covered: true}}}}},
			{Path: "pkg/b", Types: []TypeReport{{Name: "B", Methods: []MethodReport{{Name: "Run", Covered: false}}}}},
		},
		Total:   2,
		Covered: 1,
	}

	var buf bytes.Buffer
	Render(&buf, report)
	out := buf.String()

	if !strings.Contains(out, "=== pkg/a ===") {
		t.Errorf("expected package header for pkg/a, got:\n%s", out)
	}
	if !strings.Contains(out, "=== pkg/b ===") {
		t.Errorf("expected package header for pkg/b, got:\n%s", out)
	}
}
