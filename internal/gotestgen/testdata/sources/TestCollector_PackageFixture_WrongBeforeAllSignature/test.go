package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type DBFixture struct{}

func (f *DBFixture) BeforeAll(t *gotest.T) {} // wrong: should be (ctx context.Context) error
