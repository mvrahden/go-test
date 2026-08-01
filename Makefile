.PHONY: test lint build vet vuln fmt-check golangci-lint checks extension-test extension-package

# `gotest spec` exits 0 when a package's test code fails to compile, so it cannot
# tell a passing run from an uncompilable tree on its own. go vet type-checks test
# files and does catch it, so it runs first as the compile guard.
test:
	go build ./...
	go vet ./...
	go test -ldflags=-checklinkname=0 ./... ./examples/... -race
	go run ./cmd/gotest spec ./... ./examples/... -race

lint: vet
	go run ./cmd/gotest-lint ./...

vet:
	go vet ./...

build:
	go build -o gotest ./cmd/gotest

extension-test:
	cd vscode-gotest && npm test

extension-package:
	cd vscode-gotest && npx @vscode/vsce package --no-dependencies

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:" && gofmt -l . && exit 1)

golangci-lint:
	golangci-lint run ./...

checks: fmt-check vuln golangci-lint
