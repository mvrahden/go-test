package pkgload

import "golang.org/x/tools/go/packages"

// ExportSetLoad replaces the go/packages call; restore puts it back.
func ExportSetLoad(f func(*packages.Config, ...string) ([]*packages.Package, error)) (restore func()) {
	prev := load
	load = f
	return func() { load = prev }
}
