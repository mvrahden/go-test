package gotest_test

import (
	"testing"

	"github.com/mvrahden/go-test/gotest"
)

func TestNewT_wraps_testing_T(t *testing.T) {
	tt := gotest.NewT(t)
	if tt.T() != t {
		t.Fatal("T() should return the underlying *testing.T")
	}
}

func TestT_It_runs_subtest(t *testing.T) {
	var ran bool
	tt := gotest.NewT(t)
	tt.It("subtest", func(it *gotest.T) {
		ran = true
		if it.T() == nil {
			t.Fatal("It callback should receive a valid T")
		}
	})
	if !ran {
		t.Fatal("It callback should have executed")
	}
}

func TestT_It_subtest_name_appears_in_test_output(t *testing.T) {
	tt := gotest.NewT(t)
	tt.It("my_subtest_name", func(it *gotest.T) {
		if it.T().Name() != t.Name()+"/my_subtest_name" {
			t.Fatalf("unexpected subtest name: %s", it.T().Name())
		}
	})
}
