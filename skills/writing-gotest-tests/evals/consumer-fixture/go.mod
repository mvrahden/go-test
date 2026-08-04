// The replace directive delivers this repository's HEAD semantics; the
// v1.25.0 require below is overridden. `go tool gotest version` reports
// "dev (replace directive)" here — per the skill's version gate, current
// (post-v1.25.0) semantics apply.
module example.com/shopd

go 1.24.0

require (
	github.com/mvrahden/go-test v1.25.0
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/mvrahden/go-test => ../../../../

tool github.com/mvrahden/go-test/cmd/gotest
