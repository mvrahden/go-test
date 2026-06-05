package gotestruntime

import "sync"

type FixtureOnce struct {
	once sync.Once
	err  error
}

func (f *FixtureOnce) Do(fn func() error) error {
	f.once.Do(func() {
		f.err = fn()
	})
	return f.err
}
