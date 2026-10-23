package extfixtures

import "context"

type PostgresSharedFixture struct {
	ConnStr string
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PostgresSharedFixture) AfterAll(ctx context.Context) error  { return nil }

// RemoteFixture is a package fixture suites in other packages bind.
type RemoteFixture struct{}

func (f *RemoteFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *RemoteFixture) BeforeEach(ctx context.Context) error { return nil }

// Hooks lends its AfterEach to a fixture that embeds it.
type Hooks struct{}

func (h *Hooks) AfterEach(ctx context.Context) error { return nil }

// QueueSharedFixture declares a per-test hook, which shared fixtures do not
// have.
type QueueSharedFixture struct {
	Addr string
}

func (f *QueueSharedFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *QueueSharedFixture) BeforeEach(ctx context.Context) error { return nil }
