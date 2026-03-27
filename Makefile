.DEFAULT_GOAL := help

GO ?= go
PKGS ?= ./...

.PHONY: help fmt test test-race build vet bench quality-gate quality-gate-strict qa ci

help:
	@printf "Targets:\n"
	@printf "  make fmt           - Run go fmt on all packages\n"
	@printf "  make test          - Run unit tests\n"
	@printf "  make test-race     - Run tests with race detector\n"
	@printf "  make build         - Build all packages\n"
	@printf "  make vet           - Run go vet\n"
	@printf "  make bench         - Run benchmarks (single pass)\n"
	@printf "  make quality-gate  - Run standard local quality checks\n"
	@printf "  make quality-gate-strict - Include vet and formatting\n"

fmt:
	$(GO) fmt $(PKGS)

test:
	$(GO) test $(PKGS)

test-race:
	$(GO) test -race $(PKGS)

build:
	$(GO) build $(PKGS)

vet:
	$(GO) vet $(PKGS)

bench:
	$(GO) test ./... -run ^$$ -bench . -benchtime=1x

quality-gate: test test-race build

quality-gate-strict: fmt vet quality-gate

qa: quality-gate

ci: quality-gate
