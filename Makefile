.PHONY: test lint build vet vuln fmt-check golangci-lint checks extension-test extension-package

test:
	go build ./...
	go vet ./...
	go test -ldflags=-checklinkname=0 ./... ./examples/... -race
	go run ./cmd/gotest spec ./... ./examples/... -race

lint: vet
	go run ./cmd/gotest lint ./...

vet:
	go vet ./... ./examples/...

build:
	go build -o gotest ./cmd/gotest

extension-test:
	cd vscode-gotest && npm test

extension-package:
	cd vscode-gotest && npx @vscode/vsce package --no-dependencies

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./... ./examples/...

fmt-check:
	@unformatted=$$(find . -name testdata -prune -o -name '*.go' -print | xargs -r gofmt -l); \
	test -z "$$unformatted" || (echo "gofmt needed on:" && echo "$$unformatted" && exit 1)

golangci-lint:
	golangci-lint run ./... ./examples/...

checks: fmt-check vuln golangci-lint
