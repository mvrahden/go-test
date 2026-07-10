package withredundant //nolint:stdlib-test

import (
	"errors"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// === Tier 1: Error before ErrorIs/ErrorContains/ErrorAs ===

func TestErrorBeforeErrorIs(t *testing.T) {
	err := errors.New("x")
	target := errors.New("target")
	gotest.Error(t, err) // want `Error is redundant before ErrorIs`
	gotest.ErrorIs(t, err, target)
}

func TestErrorBeforeErrorContains(t *testing.T) {
	err := errors.New("x")
	gotest.Error(t, err) // want `Error is redundant before ErrorContains`
	gotest.ErrorContains(t, err, "msg")
}

func TestErrorBeforeErrorAs(t *testing.T) {
	err := errors.New("x")
	gotest.Error(t, err) // want `Error is redundant before ErrorAs`
	gotest.ErrorAs(t, err)
}

func TestErrorBeforeErrorIsWithMsg(t *testing.T) {
	err := errors.New("x")
	target := errors.New("target")
	gotest.Error(t, err, "guard") // want `Error is redundant before ErrorIs`
	gotest.ErrorIs(t, err, target)
}

// === Tier 2: NotNil before NotEmpty/Len/Contains ===

func TestNotNilBeforeNotEmpty(t *testing.T) {
	var s []int
	gotest.NotNil(t, s) // want `NotNil is redundant before NotEmpty`
	gotest.NotEmpty(t, s)
}

func TestNotNilBeforeLenPositive(t *testing.T) {
	var s []int
	gotest.NotNil(t, s) // want `NotNil is redundant before Len`
	gotest.Len(t, s, 3)
}

func TestNotNilBeforeContains(t *testing.T) {
	s := []int{1, 2, 3}
	gotest.NotNil(t, s) // want `NotNil is redundant before Contains`
	gotest.Contains(t, s, 1)
}

// === Tier 2: NotEmpty before Len/Contains ===

func TestNotEmptyBeforeLenPositive(t *testing.T) {
	var s []int
	gotest.NotEmpty(t, s) // want `NotEmpty is redundant before Len`
	gotest.Len(t, s, 3)
}

func TestNotEmptyBeforeContains(t *testing.T) {
	s := []int{1, 2, 3}
	gotest.NotEmpty(t, s) // want `NotEmpty is redundant before Contains`
	gotest.Contains(t, s, 1)
}

// === Negative cases: should NOT produce diagnostics ===

func TestNotNilBeforeLenZero(t *testing.T) {
	var s []int
	gotest.NotNil(t, s)
	gotest.Len(t, s, 0) //nolint:assertion-simplify // testing that NotNil+Len(0) is not redundant
}

func TestDifferentVariables(t *testing.T) {
	err1 := errors.New("x")
	err2 := errors.New("y")
	gotest.Error(t, err1)
	gotest.ErrorIs(t, err2, errors.New("z"))
}

func TestNonAdjacentStatements(t *testing.T) {
	err := errors.New("x")
	gotest.Error(t, err)
	_ = err.Error()
	gotest.ErrorIs(t, err, errors.New("z"))
}

func TestStandaloneAssertions(t *testing.T) {
	err := errors.New("x")
	gotest.ErrorIs(t, err, errors.New("target"))
	gotest.ErrorContains(t, err, "msg")

	s := []int{1, 2, 3}
	gotest.NotEmpty(t, s)
}
