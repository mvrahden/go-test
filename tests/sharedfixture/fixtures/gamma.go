package fixtures

import "context"

type GammaSharedFixture struct {
	Alpha   *AlphaSharedFixture
	Derived string
}

func (f *GammaSharedFixture) BeforeAll(ctx context.Context) error {
	f.Derived = "gamma-" + f.Alpha.DataPath
	return nil
}

func (f *GammaSharedFixture) AfterAll(ctx context.Context) error { return nil }
