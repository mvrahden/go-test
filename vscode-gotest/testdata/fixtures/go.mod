// Fixture corpus for the extension's end-to-end tests. Deliberately contains
// suites that fail, panic, and fail to compile, so it must never be part of the
// repo's own build: it lives under a `testdata` directory, which the go tool
// excludes from `./...` patterns. Tests resolve it through a generated go.work
// pointing at this directory and the repository root.
module gotest.fixtures

go 1.24.0
