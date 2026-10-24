GO ?= go
BIN ?= bin/ncdu-next

.PHONY: all check check-fast build artifact vet fmt test

all: check

# Tier 2 gate: what CI runs. Fails on any non-empty gofmt output, a vet error,
# a build error, or a failing (race-enabled) test.
check: build vet fmt test

build:
	$(GO) build ./...

# Build the runnable binary people run, with the revision stamped in. `build`
# above is a compile-only check and writes no binary, so an artifact left stale
# goes unnoticed; keep this current after code changes (see AGENTS.md,
# "Build artifact", and scripts/build.ps1 on Windows).
artifact:
	$(GO) build -trimpath -o $(BIN) ./cmd/ncdu-next

vet:
	$(GO) vet ./...

fmt:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

test:
	$(GO) test -count=1 -race ./...

# Tier 1 gate: fast local feedback over the guarded packages. Runs whole
# packages (not a name regex) so a guarded test cannot be silently excluded.
check-fast: fmt vet
	$(GO) test ./internal/filter/ ./internal/ui/
