package gotest_test

import (
	"errors"
	"fmt"
)

var errSentinel = errors.New("sentinel error")

type myError struct {
	Code int
}

func (e *myError) Error() string { return fmt.Sprintf("myError: code=%d", e.Code) }

type point struct{ X, Y int }
