package lintdropped

import "github.com/mvrahden/go-test/pkg/gotest"

// Shared is a helper another package may use.
type Shared struct{}

func (s *Shared) TestDeclared(t *gotest.T) {}
