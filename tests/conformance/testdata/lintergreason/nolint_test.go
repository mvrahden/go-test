package lintergreason

import "testing"

func TestNoReason(t *testing.T) {} //nolint:stdlib-test

func TestItself(t *testing.T) {} //nolint:nolint-reason,stdlib-test

func TestReasoned(t *testing.T) {} //nolint:stdlib-test // kept as a stdlib test

func TestBare(t *testing.T) {} //nolint // kept as a stdlib test

func TestForeign(t *testing.T) {} //nolint:gosec

//nolint:testify,stdlib-test // nothing below needs it
var unused = 0
