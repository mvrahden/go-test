// Package pg shares its name with another fixture package.
package pg

import "context"

// DBFixture is bound beside the other package's DBFixture.
type DBFixture struct{ Name string }

func (f *DBFixture) BeforeAll(ctx context.Context) error  { f.Name = "a"; return nil }
func (f *DBFixture) BeforeEach(ctx context.Context) error { return nil }
