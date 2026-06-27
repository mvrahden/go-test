.PHONY: test lint build vet vuln fmt-check golangci-lint checks extension-test extension-package

test:
	go run ./cmd/gotest spec ./... -race

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
