# twiceshy — build & quality gates. `make ci` reproduces the CI run locally.

GO          ?= go
COVER_FLOOR ?= 80

.PHONY: ci lint test cover cover-check build tidy doctor sec vuln secret-scan eval test-livecorpus

ci: lint cover-check sec

# Security gate — mirrors .forgejo/workflows/security.yml so `make ci` reproduces
# ALL required CI checks locally (golangci-lint already covers `go vet`). Both
# tools are optional in a bare checkout: a missing one warns + skips (CI still
# enforces it); where installed (the brain, via `make tools`) it enforces.
sec: vuln secret-scan

vuln:
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "WARN: govulncheck not installed — skipping (CI enforces it)."; \
		echo "      go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi

secret-scan:
	@if command -v gitleaks >/dev/null 2>&1; then \
		gitleaks detect --source . --config .gitleaks.toml --redact --no-banner; \
	else \
		echo "WARN: gitleaks not installed — skipping (CI enforces it)."; \
		echo "      https://github.com/gitleaks/gitleaks/releases (v8.30.1)"; \
	fi

lint:
	golangci-lint fmt --diff ./...
	golangci-lint run ./...

# Retrieval-effectiveness eval (#0005): does the validated corpus surface the
# right trap for realistic queries? Report-only (not a blocking gate — recall
# shifts as the corpus grows); run it to see the store's health.
eval: build
	$(GO) run ./cmd/twiceshy eval -corpus . -db $${TMPDIR:-/tmp}/twiceshy-eval.db

test:
	$(GO) test -race ./...

test-livecorpus:
	$(GO) test -tags livecorpus ./...

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

# Recipe-conformance doctor (cookbook@ori plugin, dotts-h/claude-skills).
# Resolve the catalog: explicit RECIPES_DIR (CI/manual) > $CLAUDE_PLUGIN_ROOT
# (set only inside a plugin command/hook, usually empty in a plain make shell) >
# the marketplace install location > a local clone. The wildcard fallback makes
# `make doctor` work in a web/CLI session with no env var.
RECIPES_DIR ?= $(CLAUDE_PLUGIN_ROOT)
ifeq ($(strip $(RECIPES_DIR)),)
RECIPES_DIR := $(firstword $(wildcard $(HOME)/.claude/plugins/marketplaces/*/plugins/cookbook) $(HOME)/claude-skills-marketplace/plugins/cookbook ../claude-skills/plugins/cookbook)
endif
doctor:
	bash $(RECIPES_DIR)/skills/recipe-doctor/scripts/run-doctors.sh .
