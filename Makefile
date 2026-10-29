GO := go
FUZZ_TIME ?= 5s

.PHONY: contracts-check contract-artifacts build test-contract test-contract-red preimplementation-check full-red require-sidecar-bin release test test-race test-fuzz test-neovim gofmt-check vet mod-tidy-check check cli-gate test-cli-helpers test-cli-helpers-race vet-cli test-cli-subject test-release-scripts

contracts-check contract-artifacts:
	@$(GO) run ./contracts/schema/check.go

build: contracts-check
	@mkdir -p bin
	@$(GO) build -o bin/sidecar ./cmd/sidecar
	@$(GO) build -o bin/sidecar-contract ./contracts/runner/cmd
	@$(GO) build -o bin/conforming ./contracts/testbin/conforming
	@$(GO) build -o bin/nonconformant ./contracts/testbin/nonconformant

require-sidecar-bin:
	@test -n "$(SIDECAR_BIN)" || (echo 'SIDECAR_BIN is required: supply the staged production executable.' >&2; exit 2)
	@test -n "$(SIDECAR_CLI_VERSION)" || (echo 'SIDECAR_CLI_VERSION is required independently of the executable.' >&2; exit 2)

test-contract: contracts-check require-sidecar-bin
	@SIDECAR_BIN="$(SIDECAR_BIN)" SIDECAR_CLI_VERSION="$(SIDECAR_CLI_VERSION)" $(GO) run ./contracts/runner/cmd

test-contract-red: contracts-check
	@$(GO) test ./contracts/runner

# Historical inventory gate, intentionally separate from the CLI-only release.
full-red: contracts-check
	@$(GO) run ./contracts/runner/cmd -required-red

preimplementation-check: build test-cli-helpers test-cli-helpers-race gofmt-check vet mod-tidy-check

# Verify the exact already-staged executable; never rebuild it here.
cli-gate: require-sidecar-bin contracts-check gofmt-check vet mod-tidy-check test-release-scripts test test-contract test-race test-fuzz

# Native staging, verification and packaging; refuses to reuse a stage.
release:
	@bash scripts/release.sh build
	@bash scripts/release.sh verify
	@bash scripts/release.sh package

check: cli-gate

test: contracts-check require-sidecar-bin
	@SIDECAR_BIN="$(SIDECAR_BIN)" SIDECAR_CLI_VERSION="$(SIDECAR_CLI_VERSION)" $(GO) test ./... -count=1 -timeout=180s

test-race: contracts-check require-sidecar-bin
	@SIDECAR_BIN="$(SIDECAR_BIN)" SIDECAR_CLI_VERSION="$(SIDECAR_CLI_VERSION)" $(GO) test -race ./... -count=1 -timeout=240s
	@set -eu; out="$$(mktemp -d "$${TMPDIR:-/tmp}/sidecar-race.XXXXXX")"; trap 'rm -rf "$$out"' EXIT; \
		$(GO) build -race -ldflags "-X github.com/trogers1/sidecar-comments/internal/cli.buildVersion=$(SIDECAR_CLI_VERSION)" -o "$$out/sidecar" ./cmd/sidecar; \
		SIDECAR_BIN="$$out/sidecar" SIDECAR_CLI_VERSION="$(SIDECAR_CLI_VERSION)" $(GO) test ./tests/cli -count=1 -timeout=300s; \
		SIDECAR_BIN="$$out/sidecar" SIDECAR_CLI_VERSION="$(SIDECAR_CLI_VERSION)" $(GO) run ./contracts/runner/cmd

test-fuzz:
	@$(GO) test ./internal/format -run '^$$' -fuzz '^FuzzParseDocument$$' -fuzztime=$(FUZZ_TIME) -parallel=1 -timeout=60s
	@$(GO) test ./internal/format -run '^$$' -fuzz '^FuzzDocumentBodyRoundTrip$$' -fuzztime=$(FUZZ_TIME) -parallel=1 -timeout=60s
	@$(GO) test ./internal/domain -run '^$$' -fuzz '^FuzzLocationRoundTrip$$' -fuzztime=$(FUZZ_TIME) -parallel=1 -timeout=60s
	@$(GO) test ./internal/domain -run '^$$' -fuzz '^FuzzRevisionDeterministic$$' -fuzztime=$(FUZZ_TIME) -parallel=1 -timeout=60s

test-neovim:
	@echo 'Neovim is deferred and is not part of the CLI-only release claim.' >&2
	@exit 2

test-cli-helpers:
	@$(GO) test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$$' -count=1 -timeout=30s

test-cli-helpers-race:
	@$(GO) test -race ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$$' -count=1 -timeout=30s

vet-cli:
	@$(GO) vet ./tests/cli

test-cli-subject: require-sidecar-bin
	@SIDECAR_BIN="$(SIDECAR_BIN)" SIDECAR_CLI_VERSION="$(SIDECAR_CLI_VERSION)" $(GO) test ./tests/cli -count=1 -timeout=180s

test-release-scripts:
	@bash scripts/release_test.sh

gofmt-check:
	@files="$$(find cmd internal contracts tests -name '*.go' -type f -print)"; test -z "$$($(GO)fmt -d $$files)" || (printf '%s\n' 'gofmt required; check is non-mutating' >&2; exit 1)

vet: contracts-check
	@$(GO) vet ./...

mod-tidy-check:
	@set -e; dir="$$(mktemp -d "$${TMPDIR:-/tmp}/sidecar-tidy.XXXXXX")"; trap 'rm -rf "$$dir"' EXIT; \
		cp go.mod go.sum "$$dir"/; cp -R contracts cmd internal tests "$$dir"/; \
		(cd "$$dir" && $(GO) mod tidy); \
		cmp -s go.mod "$$dir/go.mod" && cmp -s go.sum "$$dir/go.sum" || (printf '%s\n' 'go.mod/go.sum are not tidy; tracked files were not modified' >&2; exit 1)
