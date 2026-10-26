module github.com/mvrahden/go-test

go 1.26.0

require (
	github.com/dlclark/regexp2 v1.12.0
	github.com/fsnotify/fsnotify v1.10.1
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/mod v0.41.0
	golang.org/x/sys v0.49.0
	golang.org/x/tools v0.51.0
)

require golang.org/x/sync v0.23.0 // indirect

// The v1.34.0 tag was moved after the module proxy recorded it, so the proxy and
// GitHub serve different code. Use v1.35.0 or later.
retract v1.34.0
