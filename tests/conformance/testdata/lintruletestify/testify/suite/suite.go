// Package suite stands in for testify's suite package.
package suite

import "testing"

type Suite struct{ t *testing.T }

func (s *Suite) T() *testing.T { return s.t }

func Run(t *testing.T, s any) {}
