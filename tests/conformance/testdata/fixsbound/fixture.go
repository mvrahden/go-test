// Package fixsbound binds, from its internal and its external test package,
// a package fixture that binds a shared fixture depending on another; see
// the fixtures conformance suite.
package fixsbound

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// BaseSharedFixture writes a file in its setup and opens it in each suite's
// process.
type BaseSharedFixture struct {
	Path   string   // transferred
	Handle *os.File // assigned in Hydrate, local
}

func (f *BaseSharedFixture) BeforeAll(ctx context.Context) error {
	f.Path = filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "base-data")
	return os.WriteFile(f.Path, []byte("base-data"), 0o600)
}

func (f *BaseSharedFixture) AfterAll(ctx context.Context) error { return os.Remove(f.Path) }

func (f *BaseSharedFixture) Hydrate(ctx context.Context) error {
	var err error
	f.Handle, err = os.Open(f.Path)
	return err
}

func (f *BaseSharedFixture) Dehydrate(ctx context.Context) error {
	if f.Handle != nil {
		return f.Handle.Close()
	}
	return nil
}

// DerivedSharedFixture reads its dependency's state in its setup; it
// declares no AfterAll.
type DerivedSharedFixture struct {
	Base *BaseSharedFixture
	From string
}

func (f *DerivedSharedFixture) BeforeAll(ctx context.Context) error {
	if f.Base.Path == "" {
		return errors.New("the base fixture is not up")
	}
	f.From = "derived from " + f.Base.Path
	return nil
}

// InfraFixture is the package fixture both test packages bind.
type InfraFixture struct {
	Derived *DerivedSharedFixture
}

func (f *InfraFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *InfraFixture) AfterAll(ctx context.Context) error  { return nil }
