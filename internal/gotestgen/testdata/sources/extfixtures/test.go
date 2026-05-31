package extfixtures

import "context"

type PostgresSharedFixture struct {
	ConnStr string
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PostgresSharedFixture) AfterAll(ctx context.Context) error  { return nil }
