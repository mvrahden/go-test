package probe

import "testing"

func TestProbe(t *testing.T) {} // want `stdlib test TestProbe — consider using a gotest suite \(stdlib-test\)`

func TestAllowed(t *testing.T) {} //nolint:nilaway // want `stdlib test TestAllowed — consider using a gotest suite \(stdlib-test\)`
