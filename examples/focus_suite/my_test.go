package focussuite

import (
	_ "embed"
)

type F_FocusedTestSuite struct{}
type X_ExcludedTestSuite struct{}
type NoopTestSuite struct{}

//go:testgen echo "hello world"

//go:embed my.go
var a []byte
