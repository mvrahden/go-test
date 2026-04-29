package sharedfixture

import "context"

type RedisSharedFixture struct {
	Addr     string
	DB       int      // zero value (0) is meaningful — tests that 0 isn't omitted
	Password *string  // pointer field — tests nullable JSON round-trip (null vs "value")
	Replicas []string // tests slice/array serialization
}

func (f *RedisSharedFixture) BeforeAll(ctx context.Context) error {
	f.Addr = "localhost:6379"
	f.DB = 0
	pw := "s3cret"
	f.Password = &pw
	f.Replicas = []string{"replica1:6379", "replica2:6379"}
	return nil
}

func (f *RedisSharedFixture) AfterAll(ctx context.Context) error { return nil }
