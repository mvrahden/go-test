package lintfixbad

import (
	"errors"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func TestShadowed(t *testing.T) {
	err := errors.New("boom")
	gotest := "shadows the package"
	if err != nil {
		t.Fatal(err)
	}
	_ = gotest
}

func TestPlain(t *testing.T) {
	var err error
	gotest.True(t, err == nil)
}
