# Fix every open GitHub issue (#86 to #108), the Kuvryn Orbit port's v0.7.0 findings, and close the reworded carried stories

Status: closed 2026-11-12
Created: 2026-11-11

## Goal

Kuvryn Orbit can move all of its database code onto momo with no workarounds. Every GitHub issue open on 2026-11-11 (#86 to #111) is fixed, with a regression test, and shipped in a release:

- **Naming:** initialisms in snake_case, `-ss` singulars, and the naming preset used by plain-struct `Query`.
- **`--from-db`:** generated columns, inverse relations, and no objects of unmodeled tables.
- **Migrations:** the importer, history, partition-index inspection, and `momo.yaml` as library options.
- **Queries and expressions:** the missing operators and constructs, plus LISTEN/NOTIFY, `RawRows`, `Date` infinity, `Numeric`, and a trusted-SQL marker for momovet.

The carried plan stories that the user decided to reword (048, 049, 054, 090, 098, 099, 101 and 236) are closed with CI evidence.

## Result

committed: 34
done: 34 (20261104-048-momo-02-task-24-generated-code-size-and-compile, 20261104-049-momo-02-task-25-plan-gate-for-the-generator, 20261104-054-momo-02-task-30-field-permissions-at-query-build, 20261104-090-momo-04-task-21-staged-changes-end-to-end-postgr, 20261104-098-momo-04-task-29-cli-wiring-for-every-migrate-com, 20261104-099-momo-04-task-30-format-version-tests-across-pers, 20261104-101-momo-04-task-32-views-and-materialized-views-as, 20261104-236-momo-11-task-13-go-function-migrations, 20261111-advisory-shared-locks-are-missing, 20261111-dbdefault-no-way-to-write-false-or-0-to-a-non-zero-default, 20261111-expression-gaps-row-value-comparison-div-current-date, 20261111-from-db-writes-the-inverse-has-many-and-has-one, 20261111-fromdb-models-stored-generated-columns-as-read-only-fields, 20261111-fromdb-names-singular-ss-tables-clas, 20261111-fromdb-objects-of-unmodeled-tables-land-on-the-first-model, 20261111-history-baseline-replays-transactional-migrations-in, 20261111-history-writeimport-output-fails-migrate-validate, 20261111-inspect-models-attached-partition-indexes-as-standalone, 20261111-jsonop-root-scalar-key-delete-concat-set-with-computed-path, 20261111-lateral-unnest-of-a-column-and-distinct-aggregates, 20261111-momo-yaml-as-library-options-and-typed-migrate-state, 20261111-momovet-accepts-a-trusted-sql-marker, 20261111-naming-tosnake-splits-plural-initialisms, 20261111-nested-has-and-hasno-reuse-the-alias-momo-r, 20261111-nullcolumn-setptr-and-eqptr-for-nil-able-values, 20261111-order-by-a-predicate, 20261111-pgxdriver-has-no-listen-notify-listener, 20261111-plain-struct-query-ignores-the-naming-preset, 20261111-rawrows-exposes-the-hooked-cursor-of-a-raw-statement, 20261111-several-from-sources-and-source-returning-columns-on-update, 20261111-timeop-sub-second-intervals-and-timestamp-difference-are, 20261111-types-date-infinity-and-a-numeric-type, 20261111-union-members-cannot-have-order-by-or-limit-and-a-union, 20261111-virtual-generated-columns-read-as-stored-on-mysql-and-sqlite)
carried: 0

## Retro

### What slowed us down

- **GitHub API rate limit:** about 18 agents each polling CI used up the 5,000-call budget, and every gh call stopped until the reset.
- **Shared scratch directory:** agents downloaded CI artifacts into one shared folder and picked up each other's files. One `docs/reference.md` carried rows from another branch, and one agent committed another agent's docs.
- **Reviewers in the integration worktree:** reviewer agents ran `git checkout` in momo-integrate, which left it on a detached HEAD, and a merge failed.
- **The formatter header trap:** `launcher.sh format` prints its header on stderr, not stdout. Stripping a "header" from its output deleted the first line of `docs/capabilities.md`, `docs/ai-guide.md` and a story file.
- **Merge conflicts in shared docs:** every branch added a subsection to `docs/upgrading.md` 0.8.0, and the keep-both resolver once interleaved two sections and lost text.
- **The docs fragment ceiling:** the Go-fragment limit of 72 was full, so every new example had to be a `text` block.
- **CI congestion and outages:** up to 25 queued runs, and every push to sprint-9 cancelled the previous full run. A network outage stopped four agents mid-task.
- **A latent race:** gen's scratch directories under `gen/testdata` raced the repocheck walkers in `go test ./...` and failed the race job once.

### What we change next sprint

- **CI polling:** one watcher per agent, no more often than every 10 minutes, with a 15-minute back-off on errors.
- **Downloads:** each agent downloads artifacts into its own `mktemp -d` directory and checks the run's headSha before using it.
- **Reviewer rules:** reviewers are read-only. They use no checkout, switch, reset, stash or merge, only diff, show and grep, and the integrator checks the current branch before each merge.
- **Formatter output:** use `launcher.sh format` stdout as the file content, and never strip a line from it.
- **upgrading.md merges:** after every merge, check that each line the branch added is still there and that the file is formatter-clean (a checkupg script).
- **Pushes:** batch small integrator commits to sprint-9 so they do not cancel the full CI run.

### One adaptation worth keeping

A fresh read-only review of every green branch before it merged caught real defects that CI had passed. Examples:

- a `Lit` that dropped the time of day;
- a keyset cursor built from a predicate;
- a VIRTUAL column that `--from-db` round-tripped as STORED;
- unknown `momo.yaml` keys accepted by the library but rejected by the CLI;
- a `capabilities.md` that lost its first line.
