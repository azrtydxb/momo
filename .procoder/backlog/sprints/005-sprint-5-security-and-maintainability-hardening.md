# Sprint 5: security and maintainability hardening

Status: closed 2026-11-08
Created: 2026-11-07

## Goal

Close every finding of the 2026-11-07 security and maintainability audits (and the Scout DEEP scan) so momo is safe with untrusted input by default, its docs claim only what exists, and the code built in parallel sprints reads as one codebase before 1.0.

## Result

committed: 14
done: 14 (20261107-docs-versions-architecture-contract-go-floor, 20261107-ids-go-side-id-generators-cuid-uuid-ulid-declared-in-the, 20261107-migrations-migrate-external-extensions-and-role, 20261107-refactor-comparison-methods-and-errors, 20261107-refactor-insert-and-select-rendering, 20261107-refactor-migrate-diff-objects, 20261107-sec-driver-redaction-sensitive-args-simple-protocol, 20261107-sec-expr-like-escape-and-in-cap, 20261107-sec-migrate-history-qualification-tls-warning, 20261107-tests-consolidate-review-round-files, 20261107-tests-integration-convention-and-fuzz, 20261107-vector-support-dimensionless-vector-columns, 20261108-raw-sql-postgres-dollar-placeholders, 20261108-tx-setup-hook-and-implicit-transactions-for-rls)
carried: 0

## Retro

What slowed us down:

- Each story was reviewed on its own branch, but stale docs spread across six pages (SQLite and MySQL still called "no migrations or vectors") and three cross-story bugs only showed up in the final combined review. Examples: an empty `JoinSQL` bypassed the unfiltered-write guard, `Copy` skipped Go-side id generation, and `MapRows` failed under implicit transactions.
- Every new root dependency (mysql, mattn v1.14.53, sqlite-vec, modernc) broke the `go.sum` of each nested module that replaces the root. This cost a red CI round per merge. Guessing the tidy output by hand failed twice before `tidy-check` printed the exact diff.
- `docs/capabilities.md` conflicted on almost every dialect merge, because each branch edited the same table rows.
- Review rounds stacked up. The SQLite migrations branch needed three rounds, and each round's fixes opened new gaps: the STRICT/AUTOINCREMENT diff refusal and the WITHOUT ROWID rebuild.
- The root package's integration tests exceeded Go's 10-minute default under `-race`.

What we change next sprint:

- Sprint 6 (momo-14) adds the docs-freshness guards: generated capability and type tables with a drift check, a docs-reference-exists test, and the docscheck fix for list-indented fences. Per-dialect claims then come from one source instead of nine pages.
- When a branch adds a root dependency, read the nested-module `go.sum` change from `tidy-check` on a throwaway CI branch first, then apply it. Do not hand-edit the sums.
- The package split moves feature APIs out of the root package. That shrinks the root integration run and the merge-conflict surface.

One adaptation worth keeping:

- The final four-way area review (root, migrate, drivers, docs and tooling) over the combined sprint diff, run before opening the PR. It found 6 Important and 9 Medium issues that every per-story review had missed, and the four area fix agents worked in parallel without touching the same files.
