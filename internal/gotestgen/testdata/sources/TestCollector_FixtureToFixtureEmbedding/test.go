package testpkg

import "context"

type BaseFixture struct{}

func (f *BaseFixture) BeforeAll(ctx context.Context) error { return nil }

type DBFixture struct {
	*BaseFixture
}

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }
