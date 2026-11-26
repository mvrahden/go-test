package withnolint_file //nolint:stdlib-test // stdlib tests are this fixture's input

import "testing"

// file-level nolint suppresses all stdlib-test diagnostics in this file

func TestA(t *testing.T) {}
func TestB(t *testing.T) {}
func TestC(t *testing.T) {}
