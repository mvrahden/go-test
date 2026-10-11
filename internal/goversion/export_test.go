package goversion //nolint:stdlib-test

import "golang.org/x/tools/go/packages"

// ExportSelectedVersion reports the selection as an int, comparable to the constants below.
func ExportSelectedVersion(dir string) (string, int) {
	v, s := selectedVersion(dir)
	return v, int(s)
}

// ExportCheck is Check with the module's go-test selection stubbed.
func ExportCheck(pkgs []*packages.Package, builtWith, version string, sel int) error {
	return check(pkgs, builtWith, func(string) (string, selection) { return version, selection(sel) })
}

const (
	ExportSelected    = int(selected)
	ExportNotRequired = int(notRequired)
	ExportUnknown     = int(unknown)
)
