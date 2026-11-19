// Package q imports p.
package q

import "github.com/mvrahden/go-test/internal/pkgload/testdata/rebuilt/p"

// Make returns an Item named name.
func Make(name string) p.Item { return p.Item{Name: name} }
