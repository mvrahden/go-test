package gotest_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestEach_WithDescField(t *testing.T) {
	gt := gotest.NewT(t)

	var ran []string
	gt.Each([]struct {
		Desc  string
		Value int
	}{
		{"first entry", 1},
		{"second entry", 2},
		{"third entry", 3},
	}, func(tt *gotest.T, tc struct {
		Desc  string
		Value int
	}) {
		ran = append(ran, tc.Desc)
		if tc.Value < 1 || tc.Value > 3 {
			tt.T().Errorf("unexpected value: %d", tc.Value)
		}
	})

	if len(ran) != 3 {
		t.Fatalf("expected 3 entries to run, got %d", len(ran))
	}
	if ran[0] != "first entry" {
		t.Errorf("first ran = %q, want %q", ran[0], "first entry")
	}
}

func TestEach_WithNameField(t *testing.T) {
	gt := gotest.NewT(t)

	count := 0
	gt.Each([]struct {
		Name string
		OK   bool
	}{
		{"alpha", true},
		{"beta", true},
	}, func(tt *gotest.T, tc struct {
		Name string
		OK   bool
	}) {
		count++
		gotest.True(tt, tc.OK)
	})

	if count != 2 {
		t.Fatalf("expected 2 entries, got %d", count)
	}
}

func TestEach_WithoutDescField_UsesIndex(t *testing.T) {
	gt := gotest.NewT(t)

	count := 0
	gt.Each([]struct {
		Val int
	}{
		{10},
		{20},
	}, func(tt *gotest.T, tc struct{ Val int }) {
		count++
	})

	if count != 2 {
		t.Fatalf("expected 2 entries, got %d", count)
	}
}

func TestEach_EmptySlice_RunsNothing(t *testing.T) {
	gt := gotest.NewT(t)

	count := 0
	gt.Each([]struct{ X int }{}, func(tt *gotest.T, tc struct{ X int }) {
		count++
	})

	if count != 0 {
		t.Fatalf("expected 0 entries, got %d", count)
	}
}

func TestEach_Iterator_WithDescField(t *testing.T) {
	gt := gotest.NewT(t)

	type entry struct {
		Desc  string
		Value int
	}

	var ran []string
	for it, tc := range gotest.Each(gt, []entry{
		{"first entry", 1},
		{"second entry", 2},
		{"third entry", 3},
	}) {
		ran = append(ran, tc.Desc)
		gotest.Greater(it, tc.Value, 0)
	}

	if len(ran) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(ran))
	}
	if ran[0] != "first entry" {
		t.Errorf("first ran = %q, want %q", ran[0], "first entry")
	}
}

func TestEach_Iterator_EmptySlice(t *testing.T) {
	gt := gotest.NewT(t)

	count := 0
	for range gotest.Each(gt, []struct{ X int }{}) {
		count++
	}

	if count != 0 {
		t.Fatalf("expected 0 entries, got %d", count)
	}
}

func TestEach_Iterator_TypeSafe(t *testing.T) {
	gt := gotest.NewT(t)

	type testCase struct {
		Name   string
		Input  int
		Expect int
	}

	for it, tc := range gotest.Each(gt, []testCase{
		{"double 2", 2, 4},
		{"double 5", 5, 10},
	}) {
		gotest.Equal(it, tc.Expect, tc.Input*2)
	}
}
