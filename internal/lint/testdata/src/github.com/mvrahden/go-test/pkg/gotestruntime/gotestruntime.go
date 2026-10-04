// Package gotestruntime is a stub of the runtime for the lint fixtures.
package gotestruntime

import "testing"

func Main(m *testing.M) int { return m.Run() }
