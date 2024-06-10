package focussuite

import (
	_ "embed"
)

type F_FocusedTestSuite struct{}
type X_ExcludedTestSuite struct{}
type NoopTestSuite struct{}

//go:embed main.go
var a []byte
