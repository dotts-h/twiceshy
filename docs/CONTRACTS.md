# CONTRACTS.md — twiceshy stable promises

> The registry of seams: interfaces, routes, event vocabularies, schemas, and
> invariants other code (or other repos) relies on. Changing anything listed
> here is **deliberate** — it gets a decision record and a coordinated rollout,
> never a drive-by edit. This is an *index*, not a prose copy: each entry states
> the shape tersely and points at the source of truth.

## Internal seams

> Promises between modules inside this repo.

| seam | shape (one line) | stability | owner / source |
|------|------------------|-----------|----------------|
| — | *(none registered yet)* | — | — |

## Provides (consumed by other repos)

> Machine-checked by `fleet-doctor.sh`. Format — one bullet per contract:
> ``- `contract-id` — description · shape/schema pointer``
> The id is fleet-unique, kebab/dot style (e.g. `acme.users.api-v1`).

- *(none yet)*

## Consumes (provided by other repos)

> Same format. Every id listed here must be **provided** by exactly one sibling
> repo in the fleet's `constellation.yaml` — the fleet doctor fails otherwise.

- *(none yet)*

## Invariants

> Cross-cutting promises that aren't a single seam (determinism, ordering,
> escaping, atomicity). Each cites its decision record.

- *(none yet)*
