//go:build gotest_vendordeps

// go mod vendor reads this file, go build never does: a module that declares
// the CLI as a tool vendors the runtime its generated harness imports, and
// the CLI never links it (its init claims the runner's channel).
package main

import _ "github.com/mvrahden/go-test/pkg/gotestruntime"
