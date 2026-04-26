package gotest_test

import (
	"errors"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestMust(t *testing.T) {
	t.Run("returns value when ok is nil", func(t *testing.T) {
		result := gotest.Must(42, nil)
		if result != 42 {
			t.Errorf("expected 42, got %d", result)
		}
	})

	t.Run("returns value when ok is true", func(t *testing.T) {
		result := gotest.Must("hello", true)
		if result != "hello" {
			t.Errorf("expected hello, got %s", result)
		}
	})

	t.Run("panics when ok is error", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("expected panic, got none")
			}
		}()
		gotest.Must(0, errors.New("boom"))
	})

	t.Run("panics when ok is false", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("expected panic, got none")
			}
		}()
		gotest.Must(0, false)
	})

	t.Run("multi-return expansion", func(t *testing.T) {
		producer := func() (string, error) { return "value", nil }
		result := gotest.Must(producer())
		if result != "value" {
			t.Errorf("expected value, got %s", result)
		}
	})
}
