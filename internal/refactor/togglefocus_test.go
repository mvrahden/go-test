package refactor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToggleFocus_Suite(t *testing.T) {
	src := `package example

type FooTestSuite struct{}

func (s *FooTestSuite) TestBar() {}
func (s *FooTestSuite) TestBaz() {}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "foo_test.go")
	os.WriteFile(path, []byte(src), 0644)

	if err := ToggleFocus(path, "FooTestSuite"); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	want := `package example

type F_FooTestSuite struct{}

func (s *F_FooTestSuite) TestBar() {}
func (s *F_FooTestSuite) TestBaz() {}
`
	if string(got) != want {
		t.Fatalf("unexpected result after focus:\n%s", got)
	}

	// Toggle back
	if err := ToggleFocus(path, "F_FooTestSuite"); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != src {
		t.Fatalf("unexpected result after unfocus:\n%s", got)
	}
}

func TestToggleFocus_Method(t *testing.T) {
	src := `package example

type FooTestSuite struct{}

func (s *FooTestSuite) TestBar() {}
func (s *FooTestSuite) TestBaz() {}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "foo_test.go")
	os.WriteFile(path, []byte(src), 0644)

	if err := ToggleFocus(path, "FooTestSuite.TestBar"); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	want := `package example

type FooTestSuite struct{}

func (s *FooTestSuite) F_TestBar() {}
func (s *FooTestSuite) TestBaz() {}
`
	if string(got) != want {
		t.Fatalf("unexpected result after focus:\n%s", got)
	}

	// Toggle back
	if err := ToggleFocus(path, "FooTestSuite.F_TestBar"); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != src {
		t.Fatalf("unexpected result after unfocus:\n%s", got)
	}
}

func TestToggleFocus_ValueReceiver(t *testing.T) {
	src := `package example

type BarTestSuite struct{}

func (s BarTestSuite) TestOne() {}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "bar_test.go")
	os.WriteFile(path, []byte(src), 0644)

	if err := ToggleFocus(path, "BarTestSuite"); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	want := `package example

type F_BarTestSuite struct{}

func (s F_BarTestSuite) TestOne() {}
`
	if string(got) != want {
		t.Fatalf("unexpected result:\n%s", got)
	}
}

func TestToggleFocus_NotFound(t *testing.T) {
	src := `package example

type FooTestSuite struct{}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "foo_test.go")
	os.WriteFile(path, []byte(src), 0644)

	err := ToggleFocus(path, "NonExistent")
	if err == nil {
		t.Fatal("expected error for non-existent type")
	}

	err = ToggleFocus(path, "FooTestSuite.NonExistent")
	if err == nil {
		t.Fatal("expected error for non-existent method")
	}
}
