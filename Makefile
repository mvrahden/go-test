.PHONY: ci test lint build vet vuln fmt-check golangci-lint golangci-version checks drill extension-test extension-contract extension-package

# Must ship x/tools >= v0.46.0, the first that reads Go 1.27 export data.
# Move it with the Go version the workflows test.
GOLANGCI_VERSION := v2.13.2

# The whole Go gate, spelled once; the workflows run these same targets.
ci: checks lint test drill

test:
	go build ./...
	go vet ./...
	go run ./cmd/gotest spec $(SPEC_FLAGS) --min=70 ./... ./examples/... -race

lint: vet
	go run ./cmd/gotest lint ./... ./examples/...

drill:
	bash tests/drill/drill.sh

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
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run --allow-parallel-runners ./... ./examples/...

golangci-version:
	@echo $(GOLANGCI_VERSION)

checks: fmt-check vuln golangci-lint
