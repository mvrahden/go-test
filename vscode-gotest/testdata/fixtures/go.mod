// Fixture corpus for the extension's end-to-end tests. Deliberately contains
// suites that fail, panic, and fail to compile, so it must never be part of the
// repo's own build: it lives under a `testdata` directory, which the go tool
// excludes from `./...` patterns. Tests resolve it through a generated go.work
// pointing at this directory and the repository root.
//
// The require/replace pair mirrors examples/go.mod so that the extension's own
// CLI resolution finds this working tree's gotest (the replace-directive path)
// instead of falling back to a published release.
module gotest.fixtures

go 1.24.0

replace github.com/mvrahden/go-test => ../../..

require github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000
