.PHONY: test lint build vet extension-test extension-package

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
