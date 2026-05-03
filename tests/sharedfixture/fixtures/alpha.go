package fixtures

import (
	"context"
	"os"
)

type AlphaSharedFixture struct {
	DataPath string   // transfer: serialized to state.json
	Handle   *os.File // local: assigned by Hydrate, not serialized
}

func (f *AlphaSharedFixture) BeforeAll(ctx context.Context) error {
	tmp, err := os.CreateTemp("", "gotest-alpha-*")
	if err != nil {
		return err
	}
	_, _ = tmp.WriteString("alpha-data")
	tmp.Close()
	f.DataPath = tmp.Name()
	return nil
}

func (f *AlphaSharedFixture) AfterAll(ctx context.Context) error {
	return os.Remove(f.DataPath)
}

func (f *AlphaSharedFixture) Hydrate(ctx context.Context) error {
	var err error
	f.Handle, err = os.Open(f.DataPath)
	return err
}

func (f *AlphaSharedFixture) Dehydrate(ctx context.Context) error {
	if f.Handle != nil {
		return f.Handle.Close()
	}
	return nil
}
