package extbadfixture

import "context"

type BadFixture struct{}

func (f *BadFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BadFixture) BeforeEach(ctx context.Context)      {}
