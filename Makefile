# twiceshy — build & quality gates. `make ci` reproduces the CI run locally.

GO          ?= go
COVER_FLOOR ?= 80

.PHONY: ci lint test cover cover-check build tidy

ci: lint cover-check

lint:
	golangci-lint run ./...

test:
	$(GO) test -race ./...

cover:
	$(GO) test -race -covermode=atomic -coverprofile=coverage.out ./...

cover-check: cover
	@total=$$($(GO) tool cover -func=coverage.out | awk '/^total:/ {sub(/%/,"",$$3); print $$3}'); \
	awk -v t="$$total" -v f="$(COVER_FLOOR)" 'BEGIN { \
		if (t+0 < f+0) { printf "FAIL: coverage %.1f%% is below the floor of %s%%\n", t, f; exit 1 } \
		printf "coverage %.1f%% (floor %s%%)\n", t, f }'

build:
	$(GO) build ./...

tidy:
	$(GO) mod tidy
