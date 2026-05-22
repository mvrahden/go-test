package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type RedisSharedFixture struct{}

func (f *RedisSharedFixture) BeforeAll(t *gotest.T) {} // wrong: should be (ctx context.Context) error
