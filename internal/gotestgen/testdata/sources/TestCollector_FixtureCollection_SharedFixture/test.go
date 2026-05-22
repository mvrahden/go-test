package testpkg

import "context"

type RedisSharedFixture struct {
	Addr string
}

func (f *RedisSharedFixture) BeforeAll(ctx context.Context) error { return nil }
