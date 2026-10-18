package lintergnolint

import "testing"

func TestBare(t *testing.T) {} //nolint // kept as a stdlib test on purpose

func TestAll(t *testing.T) {} //nolint:all

func TestUnknown(t *testing.T) {} //nolint:not-a-rule

func TestUnknownBeside(t *testing.T) {} //nolint:all,not-a-rule

func TestForeign(t *testing.T) {} //nolint:gosec // another linter's rule
