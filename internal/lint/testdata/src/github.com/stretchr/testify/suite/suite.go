package suite

import "testing"

// Suite stands in for testify's suite.Suite.
type Suite struct{ t *testing.T }

func (s *Suite) T() *testing.T { return s.t }

func (s *Suite) SetT(t *testing.T) { s.t = t }

// Run stands in for testify's suite.Run.
func Run(t *testing.T, s any) {}
