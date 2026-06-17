# TECH_DEBT.md — twiceshy debt register

> Known, *deliberate* debt — each entry names what was traded away, why, and the
> trigger that should force repayment. An entry with no repayment trigger is a wish,
> not a register row. Paying an item down removes the row (git history remembers).

| # | item | why deferred | repay when |
|---|------|--------------|------------|
| M3 | `index.NextID` is a `MAX(id)+1` read, not an atomic allocation — two concurrent `record_experience` calls can be handed the same id | the write path is **propose-only**: it returns a quarantined draft for a human to open as a PR (the trust boundary), and a colliding id is caught in review and cheap to re-derive at merge | concurrent *unattended* writes become possible (auto-merge, or a non-PR write path) — then reserve/allocate the id transactionally or re-derive it at merge |
| L6/L7 | `DefaultFloor` (`2.0e-06`) is a coarse, corpus-coupled BM25 band, pinned only by a *relative* boundary test; a proportional BM25 shift could flip Similar↔Novel while the seed-fixture guard stays green | the seed corpus is tiny; an absolute/normalized band needs more data to calibrate | the corpus outgrows the seed set — replace with normalized/RRF banding (ADR-0006) |
| ADR-0006 | score-banding (explicit Similar-vs-Novel thresholds) is deferred | depends on dense retrieval + reciprocal-rank fusion landing first | issue #6 (dense retrieval, sqlite-vec + RRF) lands |
