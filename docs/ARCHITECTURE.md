# twiceshy — Architecture

> The engineering deep dive: how Git-backed experience service that feeds engineering lessons to LLM agents via MCP is put together and *why*.
> Keep it honest — when the code and this doc disagree, the code wins, so update this
> when a boundary moves. Decisions that shaped it link to ADRs (docs/adr/).

## Design goals

> The two or three load-bearing principles every module choice is justified against
> (e.g. *pure core / thin edges*, *testable without a network*, *cost is first-class*).

## Module map

> A short tree of the top-level packages/dirs and the one job each owns — entry points
> first, domain core in the middle, IO/transport edges last.

## Seams / contracts

> The interfaces that decouple core from edges (the boundaries you mock in tests). Name
> each seam, its method set, and its implementations (real + test double).

## Failure & offline behavior

> What happens when a dependency is missing or down — degraded modes, missing-file =
> defaults, atomic writes — so a first run never errors and a fault never corrupts state.

## CI/CD

> The gates that must pass to merge and what ships on a release — lint/vet, tests with
> the coverage floor, build matrix, and the release/publish path.

## Testing philosophy

> How the layers are tested (test-first? edge/invariant/fuzz/concurrency?) and where each
> layer lives — plus the rule that a fixed bug ships with the test that now guards it.
