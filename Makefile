BINARY  := reconsentry
PKG     := ./...
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: help build install test race lint fmt vet tidy clean

help: ## list targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

build: ## build the binary into ./$(BINARY)
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/reconsentry

install: ## go install the binary
	go install -ldflags="$(LDFLAGS)" ./cmd/reconsentry

test: ## run tests
	go test $(PKG)

race: ## run tests with the race detector
	go test -race $(PKG)

lint: ## run golangci-lint (install: https://golangci-lint.run)
	golangci-lint run

fmt: ## gofmt the tree
	gofmt -w .

vet: ## go vet
	go vet $(PKG)

tidy: ## tidy go.mod / go.sum
	go mod tidy

clean: ## remove build artifacts
	rm -f $(BINARY)
	rm -rf dist
