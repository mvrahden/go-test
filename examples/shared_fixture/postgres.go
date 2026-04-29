package sharedfixture

import "context"

// ConnSSL tests nested struct serialization through the JSON pipeline.
type ConnSSL struct {
	Mode     string
	CertPath string
}

// PoolHandle is a non-serializable resource created during Hydrate.
type PoolHandle struct {
	MaxConns int
	Active   int
}

type PostgresSharedFixture struct {
	DSN  string
	Port int
	SSL  ConnSSL           // nested struct — tests composite type round-trip
	Tags map[string]string // tests map serialization

	Pool  *PoolHandle // local pointer: assigned by connect() — tests pointer local field classification
	Ready bool        // local: assigned by validate() helper in Hydrate
}

func (f *PostgresSharedFixture) BeforeAll(ctx context.Context) error {
	f.DSN = "postgres://localhost:5432/test"
	f.Port = 5432
	f.SSL = ConnSSL{Mode: "verify-full", CertPath: "/etc/ssl/cert.pem"}
	f.Tags = map[string]string{"env": "test", "team": "platform"}
	return nil
}

func (f *PostgresSharedFixture) AfterAll(ctx context.Context) error { return nil }

func (f *PostgresSharedFixture) Hydrate(ctx context.Context) error {
	if err := f.connect(ctx); err != nil {
		return err
	}
	return f.validate(ctx)
}

func (f *PostgresSharedFixture) Dehydrate(ctx context.Context) error { return nil }

func (f *PostgresSharedFixture) connect(_ context.Context) error {
	f.Pool = &PoolHandle{MaxConns: 10, Active: 0}
	return nil
}

func (f *PostgresSharedFixture) validate(_ context.Context) error {
	f.Ready = true
	return nil
}
