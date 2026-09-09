.PHONY: test lint build vet vuln fmt-check golangci-lint checks extension-test extension-contract extension-package

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

extension-test: extension-contract
	cd vscode-gotest && npm test

# Re-records the CLI/extension contract and fails on drift, as CI's contract
# workflow does. A CLI change that moves the recorded output surfaces here
# instead of in a CI job the Go gate never runs.
extension-contract:
	cd vscode-gotest && npm run contract:check

extension-package:
	cd vscode-gotest && npx @vscode/vsce package --no-dependencies

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./... ./examples/...

fmt-check:
	@unformatted=$$(find . -name testdata -prune -o -name '*.go' -print | xargs -r gofmt -l); \
	test -z "$$unformatted" || (echo "gofmt needed on:" && echo "$$unformatted" && exit 1)

golangci-lint:
	golangci-lint run --allow-parallel-runners ./... ./examples/...

checks: fmt-check vuln golangci-lint
