# twiceshy — build & quality gates. `make ci` reproduces the CI run locally.

GO          ?= go
COVER_FLOOR ?= 80

.PHONY: ci lint test cover cover-check build tidy doctor

ci: lint cover-check

lint:
	golangci-lint fmt --diff ./...
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
