.PHONY: contracts-check contract-artifacts build test-contract test-contract-red preimplementation-check full-red require-sidecar-bin release test test-race test-fuzz test-neovim gofmt-check vet mod-tidy-check check

GO := go

.PHONY: test-cli-helpers test-cli-helpers-race vet-cli test-cli-subject

# Stage A only: green helper/schema self-tests, no Sidecar behavior claim.
test-cli-helpers:
	@$(GO) test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$$' -count=1 -timeout=30s

test-cli-helpers-race:
	@$(GO) test -race ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$$' -count=1 -timeout=30s

vet-cli:
	@$(GO) vet ./tests/cli

# All simple real-public-CLI cases plus explicit availability gate.
# No contracts runner, fake subject, fallback, or skip is used here.
test-cli-subject:
	@SIDECAR_BIN="$(SIDECAR_BIN)" $(GO) test ./tests/cli -run '^(TestSubject.*|TestFormatLifecycle)$$' -count=1 -timeout=120s

contracts-check contract-artifacts:
	@$(GO) run ./contracts/schema/check.go

# Pre-implementation build: only the independent black-box harness and its
# intentionally nonconformant subject are buildable; no Sidecar is produced.
build: contracts-check
	@set -e; out="$$(mktemp -d "$${TMPDIR:-/tmp}/sidecar-build.XXXXXX")"; trap 'rm -rf "$$out"' EXIT; \
		$(GO) build -o "$$out/sidecar-contract" ./contracts/runner/cmd; \
		$(GO) build -o "$$out/conforming" ./contracts/testbin/conforming; \
		$(GO) build -o "$$out/nonconformant" ./contracts/testbin/nonconformant; \
		mkdir -p bin; cp "$$out/sidecar-contract" bin/sidecar-contract; cp "$$out/conforming" bin/conforming; cp "$$out/nonconformant" bin/nonconformant

# This is a real external-binary gate. It never substitutes the test binary.
test-contract: contracts-check
	@test -n "$(SIDECAR_BIN)" || (printf '%s\n' 'SIDECAR_BIN is required: no production Sidecar exists in this pre-implementation repository.' >&2; exit 2)
	@SIDECAR_BIN="$(SIDECAR_BIN)" $(GO) run ./contracts/runner/cmd

# Green means the harness successfully rejected the deliberately bad subject.
# It is a runner self-test, not a release or complete-red-suite claim.
test-contract-red: contracts-check
	@$(GO) test ./contracts/runner

# This is the only green pre-implementation gate. It checks the independent
# harness and artifacts, but deliberately does not claim executable coverage.
preimplementation-check: build test test-race gofmt-check vet mod-tidy-check

# A future release must clear every explicit required-red pack. This target is
# intentionally non-green until their concrete fixtures exist; it never turns
# the partial seed inventory into a coverage claim.
full-red: preimplementation-check
	@$(GO) run ./contracts/runner/cmd -required-red

# Production releases require an explicitly supplied production executable;
# reporting outstanding obligations is not a release gate success condition.
require-sidecar-bin:
	@test -n "$(SIDECAR_BIN)" || (printf '%s\n' 'SIDECAR_BIN is required for a production release.' >&2; exit 2)

# This is a fail-closed production release gate. In addition to the required
# red packs, it must run an external Sidecar contract and every listed quality
# gate; no obligations report can make release or check pass by itself.
release: full-red require-sidecar-bin test-contract test-fuzz test-neovim test-race vet gofmt-check mod-tidy-check

test: contracts-check
	@$(GO) test ./...

test-race: contracts-check
	@$(GO) test -race ./contracts/runner

# There are no production fuzz targets. This target deliberately fails instead
# of reporting a harness test as fuzz coverage.
test-fuzz:
	@printf '%s\n' 'test-fuzz unavailable: production parser/domain fuzz targets do not exist in this pre-implementation repository.' >&2
	@exit 2

# A required release capability must fail explicitly, never skip to green.
test-neovim:
	@printf '%s\n' 'test-neovim unavailable: the v1 Neovim adapter is not implemented in this pre-implementation repository.' >&2
	@exit 2

gofmt-check:
	@files="$$(find contracts -name '*.go' -type f -print)"; test -z "$$($(GO)fmt -d $$files)" || (printf '%s\n' 'gofmt required; check is non-mutating' >&2; exit 1)

vet: contracts-check
	@$(GO) vet ./...

mod-tidy-check:
	@set -e; dir="$$(mktemp -d "$${TMPDIR:-/tmp}/sidecar-tidy.XXXXXX")"; trap 'rm -rf "$$dir"' EXIT; \
		cp go.mod go.sum "$$dir"/; cp -R contracts "$$dir"/contracts; \
		(cd "$$dir" && $(GO) mod tidy); \
		cmp -s go.mod "$$dir/go.mod" && cmp -s go.sum "$$dir/go.sum" || (printf '%s\n' 'go.mod/go.sum are not tidy; tracked files were not modified' >&2; exit 1)

# Kept as the production release alias for callers that historically used
# check. It inherits release's fail-closed executable and quality gates.
check: release
