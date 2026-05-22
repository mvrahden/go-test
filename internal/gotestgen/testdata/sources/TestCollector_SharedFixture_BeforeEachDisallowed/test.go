package testpkg

import "context"

type RedisSharedFixture struct{}

func (f *RedisSharedFixture) BeforeAll(ctx context.Context) error    { return nil }
func (f *RedisSharedFixture) BeforeEach(ctx context.Context) error   { return nil }
