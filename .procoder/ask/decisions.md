## Recreate the momo PgBouncer test container with the new pgbouncer.ini (story 065)

- Recreate now: `docker compose -f internal/testdb/compose.yaml up -d --force-recreate pgbouncer`, then re-run the PgBouncer integration tests against the new config (max_prepared_statements = 0, plain auth, userlist).
- Leave the running container as is; the new config is applied the next time the test database is brought up with `make db-up`.

Decision (2026-11-05): recreate now. Mounting the ini failed (Docker Desktop cannot share /Volumes/DATA), so the same settings are applied through the image environment; the container was recreated with them.

## Isolate the PgBouncer suite run in a scratch database (story 136 / 065)

The edoburu PgBouncer image cannot track `search_path` (no env variable for `track_extra_parameters`, and mounting an ini fails on this host and in CI), so pooled backends keep stale per-test schemas and `suite.RunAll` through the pooler is red.

- Route every database through the pooler (env `DB_HOST`/`DB_USER`/`DB_PASSWORD` without a database name gives a `* =` entry) in compose and CI, recreate Momo's pgbouncer container once more, and run the pooled suite in a per-run scratch database `momo_bnc_<hex>` that is dropped afterwards.
- Leave the pooler as is and schema-qualify every table name in the shared suite.
- Leave phase 2 red and carry the S-30 criterion of story 065.

Decision (2026-11-05): scratch database per run.

## v0.1.0 scope and release push

- Lock scope: merged relations plus stories 149, 154 and 150; stories 151, 152 and 155 move to 0.2; approve pushing sprint-2 and opening the PR into main once the scope is done (recommended)
- Lock that scope, but ask again before pushing
- Wait for all of momo-06 (stories 150 to 155) before release

Decision (2026-11-06): wait for all relations (stories 150 to 155) before v0.1.0.

## Where story verification runs

- Push each story branch (and sprint-2) to origin so GitHub Actions runs the full suite, race, lint and the pg16/pg17 matrix; agents run only targeted tests locally (recommended)
- Keep everything local, but cap concurrent agents at two and move GOCACHE off the full DATA disk
- Keep the current setup

Decision (2026-11-06): push branches, use CI; standing approval to push worktree-agent-* story branches and sprint-2 to origin (no PRs or merges to main without asking); local runs limited to the story's own tests.

## Stories with honestly unticked criteria before v0.1.0

- Carry 094, 097, 098, 123, 124 and 148 out of sprint 2 with their reason recorded (pre-existing code so no red state; SQLite/MySQL not built yet; CLI commands rebase/lint/rollback/new not built); they stay open in the backlog and the sprint can close (recommended)
- Tick the "failed before the implementation" criteria with a note that mutation runs stand in for the red state, close 094, 097, 124 and 148; carry 098 and 123
- Leave all of them open in sprint 2 and decide after the release

Decision (2026-11-06): carry with reasons; 094, 097, 098, 123, 124 and 148 go back to the backlog with the reason recorded, nothing ticked without evidence.

## Open the v0.1.0 relations PR (sprint-2 into main)

- Open the PR with the drafted title and body, then run the merge loop (checks, bot reviews) and ask again before squash-merging (recommended)
- Open it as a draft PR first
- Do not open it yet

Decision (2026-11-06): open the PR with the drafted title and body; ask again before squash-merging.

## Squash-merge PR #5 into main

- Squash-merge PR #5 now, delete the remote sprint-2 branch afterwards only if you want sprint-2 retired, then check that release-please's PR #3 shows 0.1.0 (recommended)
- Wait for a human review first
- Do not merge yet

Decision (2026-11-06): squash-merge PR #5 now; keep sprint-2 as the working branch; check release-please PR #3 before tagging.

## Release v0.1.0 (merge release-please PR #3)

- Edit PR #3's CHANGELOG to add a "PostgreSQL + pgvector only; SQLite and MySQL are not usable yet" note and reword the MySQL dialect line, run the pre-tag release check, then merge PR #3 so release-please tags v0.1.0, and remove release-as afterwards (recommended)
- Merge PR #3 as generated, then remove release-as
- Do not release yet

Decision (2026-11-06): fix the changelog (PG + pgvector only note, reword the MySQL line), run the pre-tag check, merge PR #3 to tag v0.1.0, then remove release-as.

## Open the docs and examples PR (sprint-2 into main)

- Open the PR "docs: guides, capability matrix and runnable examples" from sprint-2 into main with the drafted body; ask again before merging (recommended)
- Open it as a draft PR first
- Do not open it yet

Decision (2026-11-06): open the PR from sprint-2 into main with the drafted title and body; ask again before merging.

## Squash-merge PR #7 into main

- Squash-merge PR #7 now with its title as the commit subject; keep sprint-2 as the working branch (recommended)
- Wait for a human review first
- Do not merge yet

Decision (2026-11-06): squash-merge PR #7 now; keep sprint-2 as the working branch.

## Issue #8: WHERE on ON CONFLICT (design and release)

- Build it as designed: ConflictQuery.Where (conflict-target predicate), InsertQuery.Where after DoUpdate (action condition, existing row table-qualified), Column.Excluded() expression; PostgreSQL and SQLite, ErrUnsupported on MySQL; integration tests incl. the lease RowsAffected case; docs and AI guide updated; release as v0.2.0 via release-please (recommended)
- Same design, but release as v0.1.1 (release-as override)
- Only the action WHERE (skip the conflict-target WHERE)
- Do not build it yet

Decision (2026-11-06): build issue #8 as designed (ConflictQuery.Where, InsertQuery.Where after DoUpdate, Column.Excluded(); PG and SQLite, ErrUnsupported on MySQL) and release as v0.2.0 via release-please.

User direction (2026-11-06): "get it build and released, use more agents if needed, but hurry" — standing approval for issue #8: open the PR, squash-merge when green and reviewed, merge the release-please PR for v0.2.0.

## GitHub Actions: version tags or commit SHAs (Scout semgrep mutable-action-tag)

- Keep version tags per the azrtydxb CI standard; revert the SHA pins; mark the Scout finding IGNORED with a note (recommended)
- Switch Momo to SHA pins and change the repocheck tests

Decision (2026-11-06): keep version tags (azrtydxb CI standard); SHA pins reverted; Scout finding IGNORED with the standard as the reason.

## Dependabot PR #2 (pgx 5.11.0 -> 5.12.0)

- Rebase onto current main, re-run CI, squash-merge when green (recommended)
- Leave it open
- Close it

Decision (2026-11-07): rebase via @dependabot, squash-merge once the full CI matrix is green.

## When to release v0.3.0 (release-please PR #18)

- Merge the release PR now that sprint 3 is on main
- Hold the release until the kuvryn-license gaps are closed

Decision (2026-11-07): hold. v0.3.0 is released only when the gaps found by the kuvryn-license audit (issues #13 to #17 and the audit's list) are closed and the kuvryn-license migration to momo can be finished. PR #18 stays open and unmerged until then.

## Opening and merging PRs to main

- Ask before every PR and merge
- Open and merge when CI is green and reviews are addressed, without asking

Decision (2026-11-07): "keep merging, no need to ask me" — open and merge without asking; releases are a separate decision (see above).

## Finishing the kuvryn-license port and timing v0.3.0

- Finish the port in kuvryn-license-momo (store/momo), CI-only verification (recommended)
- Leave the port to the user
- Release v0.3.0 after the port runs on momo main (recommended)
- Release v0.3.0 now

Decision (2026-11-07): finish the port in kuvryn-license-momo on branch store/momo; release v0.3.0 only after kuvryn-license runs fully on momo main.

## Merge kuvryn-license PR #28 (finish the momo port) once CI is green

- Merge PR #28 into kuvryn-license main when CI is green (recommended): main deploys, start-up migrations are unchanged
- Leave PR #28 as a draft for the user to merge

Decision (2026-11-07): merge PR #28 into kuvryn-license main once CI is green.

## Scout's model provider key (scans fail with "model provider unavailable")

- Replace Scout's provider key (…4a1c) with the current fastllm key (…9f8c) and rerun the DEEP scan (recommended)
- Leave Scout's settings alone; the user updates the key

Decision (2026-11-07): replace the key. Scout's default provider now uses the current fastllm key (…9f8c); the stored …4a1c key was rejected. The models were online throughout.

## GitHub repository security settings for azrtydxb/momo (security audit M4)

- Enable private vulnerability reporting, secret scanning with push protection, and protect main (require the CI checks, no force push, no deletion) (recommended)
- Enable private vulnerability reporting and secret scanning only; leave main unprotected
- Leave the repository settings as they are

Decision (2026-11-07): all three. Private vulnerability reporting and secret scanning with push protection are enabled; main requires lint, unit, race, the integration matrix, compile-fail, govulncheck, windows, macos and the PR title check, with no force push or deletion. fuzz-smoke and bench-smoke stay optional (fuzz-smoke has a known timeout flake). Admins are exempt so release-please PRs, which get no CI runs, can still merge.

## NovaCRM adoption gaps (issues #27 to #34)

- Plan them as sprint 6 after a NovaCRM gap audit, pulling the docs/code mismatch #29 into sprint 5 (recommended)
- Add them to sprint 5 now
- Leave them in the backlog

Decision (2026-11-07): "do it" — audit NovaCRM (read-only) into .procoder/research/novacrm-gap-audit.md, pull #29 into sprint 5, plan the rest as sprint 6 once sprint 5 merges.

## PR #41 (ci/agent-branch-fast-ci): a duplicate lean-CI change opened against main

- Close #41 as superseded by sprint-5 commit 126646b (ci-agent.yml with lint, unit/compile-fail/race, fuzz smoke and PostgreSQL integration) (recommended)
- Keep #41: it is the user's own work from another session; leave it open
- Merge #41 into main and drop 126646b from sprint-5

Decision (2026-11-08): close #41 as superseded by 126646b on sprint-5.

Note (2026-11-08): #41 had in fact been merged into main on 2026-11-07, so main carried agent-ci.yml next to sprint-5's ci-agent.yml and every agent branch ran both. Merging main into sprint-5 drops agent-ci.yml, keeping ci-agent.yml (which also runs fuzz smoke and PostgreSQL integration).

## Story criteria that cannot be met: "failed before the implementation" and a local `procoder check`

Stories 068, 097, 091-093, 112, 127, 131, 132 and 133 each carry a criterion that the test failed before the code existed, plus a local gate run. Laptop test runs are forbidden, and the code was written before the tests ran on CI.

- Prove red on CI: for each story, push a throwaway branch with the implementation reverted, record the failing run id as the red evidence, then close (recommended)
- Waive both criteria for these stories with a note; the commit hook's gate run counts as the gate
- Carry the stories open until a later sprint

Decision (2026-11-08): prove red on CI — for each such story, push a throwaway worktree-agent-*-red branch with the implementation reverted, cite the failing run as red evidence, then close.

## Root-package structure and always-current docs

- Guards now, reorganisation as the first story after sprint-5 merges (recommended)
- Everything after sprint-5, as a dedicated structure-and-docs sprint
- Everything now, accepting conflict work on the in-flight branches
- Scope: API-only root (no breaking changes) (recommended) / file-prefix taxonomy only / breaking split into feature subpackages

Decision (2026-11-08): everything after sprint-5, as a breaking split. Once sprint-5 is on main, a dedicated sprint 6 moves feature APIs into subpackages (breaking imports before v1), moves non-API mechanics into internal/, enforces a file-prefix taxonomy and an architecture test, and adds the docs-freshness guards: one source for version strings, generated capability and type tables with a drift check, a docs-reference-exists test, compiled code blocks, and an apidiff docs gate. A stale-docs sweep against the real code runs in the same sprint.

## Scout's model provider key (scans fail with "model provider unavailable")

- Replace Scout's provider key (…4a1c) with the current fastllm key (…9f8c) and rerun the DEEP scan (recommended)
- Leave Scout's settings alone; the user updates the key

Decision (2026-11-07): replace the key. Scout's default provider now uses the current fastllm key (…9f8c); the stored …4a1c key was rejected. The models were online throughout.

## GitHub repository security settings for azrtydxb/momo (security audit M4)

- Enable private vulnerability reporting, secret scanning with push protection, and protect main (require the CI checks, no force push, no deletion) (recommended)
- Enable private vulnerability reporting and secret scanning only; leave main unprotected
- Leave the repository settings as they are

Decision (2026-11-07): all three. Private vulnerability reporting and secret scanning with push protection are enabled; main requires lint, unit, race, the integration matrix, compile-fail, govulncheck, windows, macos and the PR title check, with no force push or deletion. fuzz-smoke and bench-smoke stay optional (fuzz-smoke has a known timeout flake). Admins are exempt so release-please PRs, which get no CI runs, can still merge.

## apidiff-docs gate: required check on main?

- Required on main once it has passed on a real PR (recommended)
- Advisory only: runs and comments, not required
- Decide later

Decision (2026-11-09): required on main. The `apidiff-docs` job (sprint 6, story apidiff-docs-gate) is added to main's required status checks after it has passed on a real pull request (the sprint-6 → main PR). A PR changes the exported API only with a docs change or the line `Docs: no reader-visible change: <reason>`.

## Commit gate cannot lint nested Go modules

Question: the commit gate's golangci-lint runs from the root module and blocks
any commit touching a file of a nested module (`bench/`, `examples/*`,
`internal/integration`, the compat fixtures) with "main module does not contain
package". How do we land such a change?

Decision (2026-11-09): for the vecsearch merge, cherry-pick the story branch's
own bench commits (already green on ci-agent, bench-module job included) onto
sprint-6. CI lint and the bench-module job remain the check for nested modules.
The gate gap is a procoder issue to raise; do not disable golangci-lint.

Follow-up (2026-11-09): the vecop merge needs a two-line edit in
`bench/vector/topk_integration_test.go` that no existing commit carries, so a
cherry-pick is not available. Decision: fix the gate first. procoder's Go lint
will run golangci-lint per module from each module's own go.mod; until that
fix is released and installed, the vecop merge waits (parked locally on
`integrate-vecop`, not pushed) and no other non-gated landing is used.

## Land the procoder nested-module gate fix (PR azrtydxb/procoder#340)?

The fix makes the gate lint and test each nested Go module from its own
go.mod, which unblocks the parked vecop merge. It is green on procoder's Ubuntu
test job; Windows and the gate job did not run. A fresh review is in progress.

- Merge and release once the review is clean, then install the new procoder
  here and land vecop.
- Merge without a release; install from main for this session only.
- Hold the PR until it runs on macOS and Windows too.

Decision (2026-11-09): merge and release once the review is clean, then
install the new procoder here and land vecop through the fixed gate. The first
review found 3 Important issues (a package at a nested module's root still
resolves to the root module in the test runner; stub tests lack Windows skips;
a changelog entry against CONTRIBUTING), sent back for fixes before merge.

## Install procoder 3.9.5 in this environment?

procoder 3.9.5 is published (https://github.com/azrtydxb/procoder/releases/tag/v3.9.5,
tag run green on Linux, macOS, Windows and the gate). It carries the
nested-module gate fix (#340, #345). The installed plugin is 3.9.2. The parked
vecop merge needs the fixed gate to commit `bench/vector/topk_integration_test.go`.

- Update the plugin now with /procoder:update, then land vecop.
- The user updates the plugin themselves; vecop waits until they say it is done.
- Do not update yet; vecop stays parked.

Decision (2026-11-09): update now. procoder was updated to 3.9.5 with
/procoder:update, verified (version, doctor, hook probe, `check` clean on the
staged bench edit) and reloaded; the vecop bench edit then committed through
the gate (2867c19).

## Release gate for momo

Decision (2026-11-09): momo releases no longer wait on kuvryn-license or any
other application. Once sprint 6 is merged to main and green, release-please
PR #26 (0.4.0) is merged. Downstream apps handle their own upgrades using
docs/upgrading.md; work stays on momo only. Replaces the earlier rule that
releases wait for the kuvryn-license port.

## Make apidiff-docs a required check on main?

The apidiff-docs gate (story 20261108-apidiff-docs-gate) is proven on a pull
request: draft PR #51 failed on an undocumented export (run 42668537688) and
passed once the escape line was in the body (run 42668814473). Its last
criterion is that `apidiff-docs` is a required status check on main. main's
required checks today are lint, unit, race, integration, compile-fail,
govulncheck, windows, macos and title; apidiff-docs is not among them.

- I add `apidiff-docs` to main's required checks with `gh api` right after the
  sprint-6 to main PR passes it.
- The user changes branch protection themselves.
- Leave it advisory (not required) for now; carry the criterion.

Decision (2026-11-09): I add it. `apidiff-docs` is added to main's required
status checks with `gh api` right after the sprint-6 to main PR passes it, and
the story's last criterion is ticked with that evidence.

## When v0.5.0 ships

Sprint 7 carries the GitHub issues from the Kuvryn Owl, Kuvryn Orbit and
NovaCRM ports (#54 to #69) and 69 older backlog stories from the first plan.
The ports wait on the issues.

- Release v0.5.0 as soon as the #54 to #69 fixes are merged and green; the
  backlog stories finish in sprint 7 and ship in v0.6.0.
- v0.5.0 waits for every sprint-7 story.

Decision (2026-11-10): ship the issues first. When every issue story is merged
into sprint-7 and green, sprint-7 goes to main and v0.5.0 is released; backlog
stories still in flight continue on a follow-up branch and ship in v0.6.0.

## Sprint 7 backlog decisions (2026-11-10)

- **View models (stories 050, 101):** runtime guard. Views are marked
  (`momo.View`), and Insert, Update and Delete return `ErrValidation` for a view
  model at runtime. No signature change; the compile-time `WritableModeler`
  design is dropped.
- **Untyped assign with field permissions (story 054):** not built. Typed paths
  already enforce read-only and insert-only columns; adding methods to the
  exported `ColumnRef` interface would break implementers. Carried as
  superseded.
- **NaN on SQLite:** rejected with `ErrValidation` when bound, instead of being
  stored as NULL.
- **Runnable examples (S-21, story 253):** PostgreSQL only. S-21 is reworded;
  the examples demonstrate PostgreSQL features.

## Sprint 7 docs decisions (2026-11-10)

- **llms.txt at the site root (stories 256, 259):** root copy. mike publishes
  only under /dev/, /vX.Y/ and /latest/, so on every release deploy the docs.yml
  deploy job also copies the released version's llms.txt and llms-full.txt to
  the gh-pages root, so /llms.txt answers 200. The links of the root llms.txt
  point into latest/. The step runs for release tags only, not for /dev/. The
  root criterion is ticked only with deploy evidence from a release.
- **Runnable docs blocks (S-23, story 256):** PostgreSQL only, the same as the
  examples (S-21). S-23 and the matching criterion of story 256 are reworded to
  say the runnable blocks are executed against PostgreSQL. Story 253's criteria
  are reworded to the per-example PostgreSQL tests that exist, so it is no
  longer carried for that reason.

## Sprint 8 design decisions (2026-11-10, integrator; the user said to keep going without waiting, and may overrule)

- **#74, database-wide functions in `--from-db`:** by default only the
  functions that a modeled table's triggers, defaults, checks or policies
  reference are written, each on the model of the table that uses it.
  `introspect.functions: all` restores the old behaviour. Reason: the old
  default put every schema function on the alphabetically first model and
  changed its header hash on every migration.
- **#77, constant defaults:** a new `introspect.dbdefault: all` tags every
  column that has a default with `dbdefault`. The default stays "computed
  defaults only", so regenerating does not change what existing models write.
- **#79, registry SQL of `Insert.Model`:** document the value dependence in the
  registry section, and add an explicit column list to the typed insert if it
  has none.

## Carried plan stories (2026-11-11, the user, via AskUserQuestion)

- **048, 049, 054, 090, 099, 101 (plan criteria that no longer match what
  shipped):** reword each criterion to name the shipped test and design, cite
  CI evidence, and close the story. The original plan text stays in the
  story's history.
- **236, Go-function migrations:** keep them forward-only. Reword the story to
  the shipped design (`ErrGoMigrationNotReversible`, `TestRefuseGoRollback`)
  and close it.
- **098, Windows CI:** the pure-Go SQLite driver running on Windows is enough.
  Reword the criterion to say so, and close the story.
- **163 and 243, benchmark results:** stay open until the user commits a
  `make bench` reference run from dedicated hardware.

## Sprint 9 design decisions (2026-11-11, the user, via AskUserQuestion)

- **#90, an explicit zero for a `dbdefault` column:** allow `dbdefault` on a
  pointer field of a NOT NULL column. nil uses the database default; a
  non-nil value is always written, including false and 0.
- **#106, momovet's trusted-SQL marker:** a `//momovet:ok <reason>` line
  directive on the call line. The reason is required, and a directive
  without one is itself reported. No new API type.
- **#108 (integrator, from the issue's expected behaviour):** objects of a
  table that is skipped or not modeled are not generated, and there is no
  opt-in.
- **#93 (integrator, from PostgreSQL semantics):** a DML body in `With`
  without `Returning` is accepted, since PostgreSQL allows a data-modifying
  CTE that nothing reads. The docs are corrected to say `Returning` is needed
  only when the outer query reads the CTE.
- **#97 (integrator, from the issue's `COALESCE(col, 'none')` example):**
  `momo.Lit` accepts strings, rendered with doubled quotes. A string with a
  backslash or a NUL byte is refused with `ErrValidation` on every dialect.
  A `time.Time` must be exactly midnight UTC and renders as `DATE '...'`;
  any other time is refused rather than truncated.
- **#107/#100 (integrator, from the review):** PID, OID, SSID and GUI join the
  initialism list, so `PIDs` maps to `pids`, not `p_ids`. The singularizer
  keeps an `-us` word unless it is a known plural of a `-u` word (menus, emus,
  gnus, haikus, tutus, kudus), and keeps listed singular `-is`, `-as` and `-ns`
  words (analysis, alias, news and others). There is no blanket `-as` or `-is`
  rule, so `areas` still gives `Area`.
  An upper-case run before a lone `s` is split so that every piece is a
  list word when that is possible (`HTTPIDs` gives `http_ids`); otherwise the
  longest list-word suffix wins.
- **#111 (integrator, from the review):** a Go model declares a VIRTUAL
  generated column with a `virtual` tag option next to `generated:<expr>`, or
  with a `MomoConfig` hook, and `--from-db` writes it. A change between STORED
  and VIRTUAL is refused with "write a custom migration" on MySQL, as on
  PostgreSQL, and is no longer a DROP and ADD. SQLite rebuilds the table.

## Issue #114: arithmetic on types.Numeric (2026-11-13, integrator)

- **Constraint:** `sqlfn.Numeric` also admits `types.Numeric`, by exact type,
  so `sqlfn.Add`, `Sub`, `Mul`, `Div` and their `Expr` forms compile on a
  `momo.Column[types.Numeric]`. sqlfn imports types; there is no cycle.
- **Exactness:** `types.Numeric` is TEXT on SQLite and MySQL, where `+` would
  go through floating point. Arithmetic on it is PostgreSQL only: on any other
  dialect the build fails with an error wrapping `momo.ErrUnsupported` that
  names the operator.
- **Other constrained functions:** `NullIf` takes `types.Numeric` and renders
  the decimal as a numeric literal (`'0'::numeric`) on PostgreSQL, because
  `momo.Lit` does not accept it; it is `ErrUnsupported` on SQLite and MySQL and
  `ErrValidation` for an invalid decimal. No other function uses the constraint.
- **Helpers:** `types.NumericFromInt64` and `types.NumericFromFloat64`. The
  float helper writes the shortest round-trip decimal (`strconv.FormatFloat`
  with 'f' and -1) and returns an error for NaN and the infinities; its doc
  warns that a float64 is already binary.

Decision (2026-11-13, integrator): as above.

## Issue #115: momotest history table in the test schema (2026-11-13, integrator)

- **Cause:** momotest connects with `search_path=<test schema>,public`; the
  migrator found `public.momo_migrations` through the path, saw everything
  applied and created nothing in the test schema.
- **Fix:** a new PostgreSQL dialect option, `postgres.WithHistorySchema(schema)`,
  pins the history table: the lookup is `to_regclass('"schema".momo_migrations')`
  and a new table is created in that schema. Every other history statement
  already uses the reference `HistoryTable` returns, and the advisory-lock key is
  derived from it. Unset, nothing changes. momotest passes the test schema (and
  the template schema for `NewTx`).
- **Guard:** after Apply, when an up migration contains `CREATE TABLE` and the
  test schema holds no table besides `momo_migrations`, momotest fails naming
  the schema. A migration set with no `CREATE TABLE` (functions, data) is not
  checked, to avoid false failures.
- **IF NOT EXISTS:** unqualified CREATE TABLE/INDEX/SEQUENCE IF NOT EXISTS and
  CREATE [OR REPLACE] FUNCTION/VIEW create in and check only the first schema of
  the path, so they are unaffected.
- **DROP/ALTER IF EXISTS (review follow-up):** the pool that applies the
  migrations (applyTo and the template build) gets `search_path=<schema>` alone
  when `WithExtensions` lists no extension, and `<schema>,public` otherwise;
  the test's runtime pool keeps `<schema>,public`. The hole (an unqualified DROP
  or ALTER ... IF EXISTS reaching `public`) remains only with extensions listed.
  No SET LOCAL.
- **Release:** exported API (`postgres.WithHistorySchema`, `New(opts ...Option)`),
  so a feat, 0.10.0.

## Issue #120: updated_at on touch and upsert (2026-11-13, integrator)

- **Touch:** an `Update` with no `Set` on a model with an `updated_at` column
  is valid and renders `UPDATE ... SET updated_at = <now> WHERE ...`, the same
  bound clock value `Update` uses. It passes every guard of a normal update
  (`ErrUnfilteredMutation`, version, interceptors). On a model without
  `updated_at` it keeps `ErrValidation` ("no columns to update").
- **DoUpdate bump:** the conflict branch of `Upsert` and of a plain `Insert`
  with `OnConflict` and `DoUpdate` always sets `updated_at` to the statement
  clock, as a bound value like `Update` (review fix: `EXCLUDED.updated_at` is
  the struct's own value and left a loaded row with an old `UpdatedAt`
  unbumped, also in shipped `Upsert`). It does not depend on the INSERT:
  `Omit` and a `FromSelect` without the column change only the insert branch,
  and an `Upsert` with an empty default set renders a touch-only `SET`. A plain
  `Insert` with `DoUpdate()` and no assignment stays an error. `DoNothing` does
  not bump. The insert branch is unchanged. The clock is read once into
  `InsertQuery.clock`, so chunks of a bulk insert share it.
- **Explicit set:** an explicit `Set` or `SetExcluded` of `updated_at` in
  `DoUpdate` is refused with `ErrValidation` and the `Update` wording
  ("cannot be set directly"). This is a breaking change, documented in
  `docs/upgrading.md`.

Decision (2026-11-13, integrator): as above.

## Issue #118: JSON null vs SQL NULL (2026-11-13, integrator)

- **Constraint:** a nullable JSON column read into `*types.JSON[T]` keeps SQL
  NULL and the JSON literal null apart. SQL NULL gives a nil pointer; the JSON
  null gives a non-nil `*types.JSON[T]` whose `IsNull` is true. This is the
  issue's option 2; no `NullJSON` type is added.
- **State:** `types.JSON[T]` gains a private marker set by `Scan` on a stored
  `null` (surrounding white space ignored) and by `types.JSONNull`. `Value`
  writes `null` while the marker is set and `V` is the zero `T` or still
  encodes as `null` (`json.RawMessage("null")`), so a read JSON null written
  back stays JSON null. A nil `*JSON[T]` still writes SQL NULL. A direct
  `V = <zero>` keeps the null; `Set` (or a fresh `JSON[T]{V: v}`) ends it. The
  pgx driver recognises the family through `types.JSONNuller`, an interface
  only `JSON[T]` satisfies.
- **Layer:** the cause was pgx, whose scan plan for a pointer to a pointer on
  json and jsonb sets the pointer to nil for the JSON null. The pgx driver's
  `Rows.Scan` (and the buffered rows of bulk mode) now scans a `**T` destination
  through `sql.Scanner` when `*T` has `IsNull`. SQLite, MySQL and the
  `FromSQLDB` path go through database/sql, which already scans a non-NULL value
  into a new pointer, so they needed no change. Those columns are `TEXT`, and
  any stored text that is `null` is the JSON null.
- **Non-pointer fields:** unchanged. SQL NULL into a plain `types.JSON[T]` is
  an error; the JSON null reads as the zero `T` with `IsNull` true.
- **Breaking:** code that relied on a JSON null reading as a nil pointer must
  test `IsNull`. Unkeyed `types.JSON[T]{v}` literals no longer compile (the
  struct has a private field); use `types.JSON[T]{V: v}`.

Decision (2026-11-13, integrator): as above.

## Issue #119: typed date arithmetic in timeop (2026-11-13, integrator)

- **API:** `timeop.AddDays(d, n int)` and `timeop.AddDaysExpr(d, n)` return a
  `timeop.DateExpr` (an `Expression[types.Date]`); `timeop.DaysBetween(a, b)`
  returns a `timeop.DaysExpr` (an `Expression[int32]`, a minus b in whole
  days). Both carry the comparisons of their type, so `LeExpr` needs no
  `momo.Typed`.
- **Rendering:** PostgreSQL `(d + n)` and `(a - b)`, with a bound date cast to
  `date` and a bound count cast to `integer` (an untyped parameter next to a
  date is ambiguous between integer and interval). MySQL `DATE_ADD(d, INTERVAL
n DAY)` and `DATEDIFF(a, b)`. SQLite, which stores a date as `YYYY-MM-DD`
  text, `date(d, '+n days')` and `CAST(julianday(a) - julianday(b) AS INTEGER)`.
- **Infinity:** PostgreSQL keeps `infinity + n` and refuses `infinity - date`
  with a database error. SQLite and MySQL have no special dates, so a bound
  `types.Date` with Infinity or NegInfinity fails the build with
  `ErrValidation` instead of returning NULL. A count outside int32 is
  `ErrValidation`.

Decision (2026-11-13, integrator): as above.

## Parity: momo migrate CLI on SQLite and MySQL (2026-11-14, integrator)

- **Dialects:** `momo migrate --dialect sqlite|mysql` runs every subcommand
  with `migrate/sqlite` and `migrate/mysql`. MySQL and MariaDB stay one dialect
  name, `mysql` (the name `dialect.For` knows); `--dialect mariadb` is refused
  with a message saying so. The server flavor comes from `VERSION()`: the CLI
  opens as MySQL and rebuilds the driver as MariaDB when the server says so.
- **Drivers:** SQLite uses `driver/sqlitepure` (modernc, no cgo), so `go
install` of `cmd/momo` needs no C toolchain; it has no vector functions,
  which a migration does not need. MySQL uses `driver/mysqldriver` with a pool
  of at most 4 connections. No new root dependency: all three are already in
  the root `go.mod`. The bench module, which imports the cli, gained their
  indirect requirements in its `go.mod` and `go.sum`, taken from CI's
  tidy-check output.
- **URLs:** SQLite takes a file path, a `file:` URI (passed on) or
  `sqlite:///abs.db` / `sqlite://rel.db` (a query string becomes a `file:`
  URI); a memory database and other schemes are refused. MySQL takes
  `mysql://` or `mariadb://` (`?socket=` for a unix socket, other options are
  go-sql-driver parameters) or a go-sql-driver DSN; a URL without a database is
  refused. Errors never quote the URL. The URL password is redacted as before.
- **Dialect-specific behavior surfaced:** a failed apply or rollback on MySQL
  exits 3 with a hint about the dirty state and `resolve`; SQLite stays
  transactional. TLS warning for a remote MySQL host without `tls=true`;
  none for SQLite. `migrate.external_extensions` (and the SQLite timeouts)
  warn on a dialect they do not apply to instead of failing, because the keys
  are shared by every dialect of one `momo.yaml`. `--lock-wait` works on all.
  There is no CLI unlock for a SQLite lock row left by a dead process; the
  docs say to delete the row.
- **Models:** the desired schema is built with the chosen dialect first, so the
  column types are that dialect's (before, `dialects[0]` or PostgreSQL). With
  `dialects: [sqlite, postgres]` and `--dialect postgres` the diff now has
  PostgreSQL types: a fix, noted in upgrading.md.
- **SQLite lock wait:** a busy database (another connection's write lock held
  past the driver's busy timeout) is `migrate.ErrLocked` in `migrate/sqlite`,
  so `--lock-wait` retries.
- **Type map:** `gen/typemap` gains a MySQL/MariaDB branch: integers (unsigned
  too), `bool` as `tinyint(1)`, `varchar(n)` or `text`, `blob`, `datetime(6)`,
  `char(36)` for UUID, `json` for `types.JSON`, `decimal(p,s)` for a
  `types.Numeric` with a precision tag, `vector(N)` for `vector.Vector`.
  `types.Date` and an untagged `types.Numeric` stay text, as their codecs
  store them. One stored format does change on MySQL: a `types.Numeric` with a
  precision tag is `decimal(p,s)` where the library produced text, so a schema
  generated earlier gets an ALTER on its next diff (upgrading.md). A string
  without `size:N`, or a blob or json column, in a key is refused at generate
  time on MySQL (error 1170 would otherwise stop a migration half-way);
  decimal precision and scale are validated (p <= 65, s <= 30, s <= p).
- **Docs:** the capabilities row comes from the dialects named in the CLI's
  `dialectImpl`, so it follows the switch.

## Parity: ILIKE, NULLS ordering and FILTER emulation (2026-11-14, integrator)

- **Question:** momo refused `ILike` on SQLite, MySQL and MariaDB, NULLS
  FIRST/LAST on MySQL and MariaDB, and the aggregate FILTER on MySQL and
  MariaDB with `ErrUnsupported`. Emulate them, or keep refusing?
- **ILike:** the old refusal was a deliberate choice, made so as not to imply
  Unicode case folding SQLite does not perform. Decided to implement it anyway
  and document the difference prominently in the `momo.ILike` doc comment: SQLite,
  MySQL and MariaDB render `LOWER(x) LIKE LOWER(?) ESCAPE '!'`. SQLite folds
  ASCII only; MySQL follows the column collation; no `_ci` or `_ai` collation is
  forced, because `_ai_` is accent-insensitive and PostgreSQL `ILIKE` is not; an
  index on the column is not used.
- **NULLS FIRST/LAST on MySQL:** `(e IS NULL) DESC|ASC, e ASC|DESC`, exact. `e`
  renders twice, so its bound arguments appear twice, in order. `After` over a
  nullable key stays `ErrUnsupported` (unchanged); the outer `OrderBy` of a
  union with a NULLS term is `ErrUnsupported` on MySQL, because an expression
  is not allowed there on every dialect.
- **FILTER on MySQL:** `agg(CASE WHEN p THEN x END)` and
  `COUNT(CASE WHEN p THEN 1 END)`, exact for aggregates that skip NULLs, also as
  window functions. Aggregates that keep NULLs (`agg.JSONB`, array aggregates,
  any `agg.Func` outside count, sum, avg, min, max and the std and var family)
  stay `ErrUnsupported` with a message that gives the reason.
- **Flags:** `dialect.Dialect.ILike` and `NullsOrdering` keep meaning native
  syntax, not availability, so they stay false on MySQL (`ILike` also on
  SQLite) while the features work. The generated capability table labels the two
  rows as native, and the feature table says what is emulated.
- **Breaking:** none in the type system. Code that relied on `ErrUnsupported`
  from these calls now gets results.

## Issue #128: --check compares the body, not only the header hash (2026-11-14, integrator)

- **Behaviour:** when the regenerated body of a `generate --from-db` model file
  equals the file's body byte for byte (everything after the header line, line
  endings normalised to LF as the plain generator does), `--check` reports the
  file as current although the header hash differs. A plain run rewrites only
  line 1 with the correct hash (action `header`, printed as `header: <file>`),
  writes no `.new` file and reports no conflict.
- **Unchanged:** a body that differs is still stale or a conflict. A file with
  no header or a malformed one keeps its handling. Plain `momo generate` does
  not read or write the header and is not changed.
- **Not chosen:** refreshing the header from `momo generate` (the issue's
  alternative).

## Issue #131: Greatest/Least, Floor/Ceil and jsonop.TypeOf (2026-11-14, integrator)

- **API:** `sqlfn.Greatest` and `sqlfn.Least` over expressions
  of an `Ordered` type (cmp.Ordered, time.Time, types.Date, types.Numeric);
  `sqlfn.Floor` and `sqlfn.Ceil` over a `Fractional` type (floats and
  types.Numeric); `jsonop.TypeOf` returning an `Expression[string]`.
  Round and Trunc are not added: PostgreSQL's `round(double precision)` rounds
  half to even and `round(numeric)` half away from zero, so one signature could
  not give one result on every dialect. Floor and Ceil take no integer type
  because PostgreSQL's `floor(integer)` is double precision and would not scan
  into the same Go type. `jsonop.TypeOf` takes the same
  `Expression[types.JSON[T]]` as the other `jsonop` functions, so a
  `types.JSON[json.RawMessage]` column works with no separate overload.
- **NULL semantics of Greatest and Least:** identical on every dialect, the
  PostgreSQL rule (NULL arguments are ignored, the result is NULL only when all
  are NULL). MySQL, MariaDB and SQLite return NULL for any NULL argument, so
  there each argument `x` is rendered as `COALESCE(x, the other arguments)`
  inside `GREATEST`/`LEAST` (MySQL, MariaDB) or the scalar `max()`/`min()`
  (SQLite). Cost: n arguments render n*n operands, and a non-deterministic
  argument is evaluated several times; documented in the doc comment.
- **Floor and Ceil per dialect:** `floor`/`ceil` on PostgreSQL, MySQL and
  MariaDB. SQLite is rendered without `floor()`: the mattn driver the CI uses
  has no math functions without its `sqlite_math_functions` build tag
  (checked in go-sqlite3 v1.14.53, the repository sets no such tag), while
  modernc enables `SQLITE_ENABLE_MATH_FUNCTIONS`. The emulation is a CASE over
  `CAST(x AS INTEGER)` and returns REAL; floats from 2^52 are whole and pass
  through, which keeps the cast away from the int64 limits.
- **types.Numeric:** Greatest, Least, Floor and Ceil over `types.Numeric` are
  PostgreSQL only and fail with `ErrUnsupported` ("PostgreSQL only") on SQLite
  and MySQL, as the `sqlfn` arithmetic does: it is TEXT there, which orders
  lexically and would floor through floating point.
- **TypeOf per dialect:** PostgreSQL `jsonb_typeof(c)`. SQLite
  `CASE json_type(c) ...` (integer, real to number; text to string; true, false
  to boolean). MySQL and MariaDB `CASE JSON_TYPE(c) ...` (INTEGER, UNSIGNED
  INTEGER, DOUBLE, DECIMAL to number; BOOLEAN, STRING, OBJECT, ARRAY, NULL
  lower-cased). All give the six PostgreSQL names; SQL NULL stays SQL NULL and
  the JSON literal null is the text `null`. A MySQL type a stored document
  cannot have (DATE, OPAQUE) maps to NULL.

## Parity: MySQL and MariaDB write read-back and upsert targets (2026-11-14, integrator)

- **MariaDB read-back:** `CreateGraph`, the identity read-back of a single
  insert and `FindOrCreate` check `InsertReturning` as well as `Returning`, so
  MariaDB reads generated keys with `INSERT ... RETURNING`. The bulk insert of
  graph children uses one multi-row `RETURNING` statement.
- **MySQL read-back:** `CreateGraph` is allowed on MySQL. Every row is inserted
  one statement at a time inside the graph's transaction and its key is read
  with `LAST_INSERT_ID`. A lock-mode 0 or 1 shortcut (one multi-row statement,
  keys first + i*increment, read from `@@innodb_autoinc_lock_mode`) was built
  and removed: CI's MySQL servers all run mode 2, so it never ran against a
  real server, and the probe could fail behind proxies. Keys are never guessed,
  `ON DUPLICATE KEY UPDATE` never reads keys, a plain multi-row `Exec` still
  writes no keys back, children mixing zero and explicit keys are
  `ErrValidation`, a non-integer generated key is `ErrUnsupported`, and the
  rows share one statement clock.
- **FindOrCreate on MySQL and MariaDB:** the brief proposed a no-op
  `ON DUPLICATE KEY UPDATE` plus the affected-rows count to tell created from
  found. The driver sets `clientFoundRows`, under which an inserted row and an
  untouched existing row both report 1 (and MariaDB's `RETURNING` returns the
  row either way), so that signal does not exist. `FindOrCreate` instead runs in
  a transaction (a savepoint on a `Tx`) as a plain insert; on a unique violation
  it re-selects by key with a shared-lock read (`FOR SHARE`, `LOCK IN SHARE
MODE` on MariaDB). A shared lock reads the latest committed row at any
  isolation level, and shared locks do not conflict with each other, so losers
  of the same race do not block one another (`FOR UPDATE` would). If the first
  inserter rolls back, the waiting losers can still deadlock (1213). On a
  caller's `Tx` InnoDB keeps the locks taken after a savepoint until the outer
  transaction ends. Cost: a failed insert plus a select when the row exists,
  and one burned auto-increment value. A violation on another unique key stays
  `ErrUniqueViolation`; the Go-side key still wins for a new row.
- **Upsert target:** on MySQL and MariaDB a target is accepted when it is a unique
  key of the table and every other unique key is made only of auto-increment
  columns the insert leaves to the database (an auto-increment primary key plus
  `UNIQUE(email)`, key omitted). Those keys take a fresh value, so
  `ON DUPLICATE KEY UPDATE` can act only on the target. A Go-side id, an
  identity written by the statement, an expression key, a composite key
  with an ordinary column, and a model without a schema stay `ErrUnsupported`,
  and the message says why.
- **Upsert column list:** the conflict-target rule decides "the insert leaves
  the auto-increment column out" from the column list the statement writes (the
  resolved model columns, the `Values` columns or the `FromSelect` targets), not
  from builder state.
- **Not done:** a multi-row `Exec` on MySQL still returns no keys.

## Parity: --from-db on MySQL and MariaDB (2026-11-14, integrator)

- **Scope:** `momo generate --from-db` reads MySQL 8.4, MySQL 9.7 and MariaDB
  11.8 through the existing `migrate/mysql` inspector and the `mysqldriver`
  package; `fromdb.Generate` accepts the MySQL dialect. The `momo migrate`
  command is not touched: only `--from-db` opens a MySQL engine.
- **Column types:** the exact column type goes in a `type:` tag (the generator
  has no MySQL type table, which the `momo migrate` CLI story adds); a varchar
  uses `size`. `tags.SplitOptions` keeps a comma inside parentheses in one
  option so `decimal(p,s)` and enum and set value lists can be written.
- **enum and set:** a Go `string` with the exact column type in the `type:` tag.
  No check constraint and no skip: both would make a database rebuilt from the
  models differ from the original. The value list stays in the database.
- **Defaults:** `dbdefault` and a pointer field for a NOT NULL bool or number
  column with a non-zero constant default on MariaDB only (it has INSERT
  RETURNING); on MySQL 8.4 and 9.7 a plain field and a note, because an insert
  with a nil `dbdefault` field is refused there. No `dbdefault` for a computed
  default on either. `time` maps to `string`. Every `tinyint(1)` bool gets a
  note: a stored value other than 0 or 1 fails to scan.
- **Reported, not modeled:** ON UPDATE and INVISIBLE columns (warnings), a
  prefix PRIMARY KEY (an error), prefix indexes (a warning). A foreign key to a
  non-primary unique column is written as `c.ForeignKey`.
- **Left out with a warning:** prefix indexes.
- **Round trip:** the integration-mysql jobs build a schema with raw DDL, run
  `--from-db`, load the models, diff them against an empty database, apply and
  compare the two inspections.

## Issue #129: Sum and Avg over integer and Numeric columns (2026-11-14, integrator)

- **Shape:** `Sum[T Number]` is unchanged, because widening `Number` would make
  `Sum[int32]` return `Agg[int32]`, the wrong type, and a sum of int4 does not
  fit int32. New functions instead: `SumInt[T Integer]` returning
  `Agg[int64]`, and `SumNumeric[T Exact]` and `AvgNumeric[T Exact]` returning
  `Agg[types.Numeric]`. `Avg` becomes `Avg[T Averageable]` returning
  `Agg[float64]`; a call `momo.Avg` with a column still infers T, so existing
  callers compile.
- **Casts:** an aggregate carries an optional PostgreSQL cast that wraps the
  call together with its FILTER and OVER clause (`CAST(SUM(x) FILTER (...) OVER
(...) AS bigint)`). PostgreSQL: `SumInt` casts to `bigint` (SUM of bigint is
  numeric), `Avg` casts to `double precision` unless the column is a float.
  MySQL, MariaDB and SQLite get no cast for `SumInt` and `Avg`: database/sql
  reads an integral DECIMAL into an int64 and a decimal into a float64, and a
  sum past int64 then fails instead of saturating, which `CAST(... AS SIGNED)`
  would do silently.
- **Exact:** `SumNumeric` and `AvgNumeric` cast an integer argument to
  `numeric` on PostgreSQL (so SUM over bigint cannot overflow). A `types.Numeric`
  column is TEXT on MySQL and MariaDB, which would sum as DOUBLE, so it is cast
  to `DECIMAL(65,30)` (30 decimal places, trailing zeros kept). SQLite: an
  integer column sums exactly (an overflow is an error), but a TEXT Numeric sums
  as a float and AVG is always a float, so those build with `ErrUnsupported`.
- **Empty set:** all of them are NULL over zero rows, as `Sum` was; read into
  a pointer. `types.Numeric.Scan` already takes int64, float64, string and
  []byte, so no codec change.
- **Widths:** `Integer`, `Averageable` and `Exact` also take `~int` and `~int8`.
- **MySQL limits:** a value or sum beyond `DECIMAL(65,30)` (35 integer digits)
  saturates or warns rather than erroring; NaN and Infinity cannot be stored.
  Numeric results differ in trailing zeros across dialects: compare numerically.
- **SQLite message:** the `ErrUnsupported` text names the aggregate and points
  to `Avg` (float64) or `SumInt` (integers).
- **API diff:** `Avg` turning generic is an apidiff incompatible change only for
  code that used `momo.Avg` as an uninstantiated function value.

## Parity: real MySQL and MariaDB integration coverage (2026-11-14, integrator)

- **Gap:** many capability rows said "golden-tested only", "not covered by a
  MySQL integration test yet" or "runs in the shared suite only where that
  suite covers it", because `testopen.Each` opens PostgreSQL and SQLite only.
- **Harness:** `testopen.EachWithMySQL` is `Each` plus the subtests `mysql84`,
  `mysql97` and `mariadb118` (dialect `MySQL`, tell the servers apart with
  `db.Dialect().Flavor`); `testopen.EachMySQL` runs the three alone, for a test
  written in MySQL SQL. `Each` is unchanged, so a test opts in one at a time and
  nothing unrelated starts failing. Each server skips when its URL is unset and
  fails under `MOMO_REQUIRE_INTEGRATION=1` for the dialects the job requires
  (`MOMO_INTEGRATION_DIALECTS`), which is what the `integration-mysql` jobs set,
  so a run that passes ran those tests. `relfixture.EachWithMySQL` adds the
  relation fixture, with a MySQL DDL (VARCHAR key columns, table-level foreign
  keys, because MySQL ignores an inline `REFERENCES`).
- **Audit result:** the shared suite already covered keyset pagination (mixed
  directions, nullable key), joins, aggregates, subqueries, CTEs, locking and
  writes on all three servers; the docs rows saying otherwise were stale. Not
  run on a MySQL-family server before: JSON operators, windows, streaming,
  `LATERAL`, the aggregate FILTER refusal, typed predicates beyond the suite,
  projections, bulk-insert chunking, `LoadPerParentLimit`, graph writes, select
  aliases, `sqlfn` helpers, `agg`, row values, `CURRENT_DATE` and the `timeop`
  fields, truncations and intervals. Each now has a test that runs on MySQL 8.4,
  MySQL 9.7 and MariaDB 11.8.
- **Bugs found:** `jsonop.Contains` rendered `CAST(? AS JSON)`, which MariaDB
  rejects (error 1064); MariaDB now gets `JSON_CONTAINS(col, ?)`.
  `timeop.AddInterval` and `SubInterval` over a bound time returned text on
  MariaDB; the operand is now `CAST(? AS DATETIME(6))`. Both are in
  upgrading.md under 0.12.0.
- **Graph writes:** `CreateGraph` and `FindOrCreate` work on both servers
  (#135); a small test asserts exact keys and find-or-create on all three.
- **Unsupported, now documented and asserted:** `LATERAL` is MySQL only;
  `Explain`, `agg.Array`, ordered or distinct JSON aggregates, `unnest`,
  `generate_series` and `DISTINCT ON` are `ErrUnsupported`.
- **Not covered:** the one-million-row streaming heap bound (its seed SQL is
  PostgreSQL and SQLite only) and the CreateGraph behaviour tests that rely on
  PostgreSQL-only SQL.

Decision (2026-11-14, integrator): as above.

## Issue #127: explicit identity values on Insert (2026-11-14, integrator)

- **API:** `InsertQuery.Explicit(cols ...ColumnRef)` is an opt-in that puts the
  named identity, serial or auto-increment columns into the INSERT column list
  with the value the source carries. Without it nothing changes: such columns
  are left out and a non-zero struct value is dropped silently, which the
  Insert docs now say prominently.
- **Rendering:** PostgreSQL adds `OVERRIDING SYSTEM VALUE` only when an
  explicit column is `GENERATED ALWAYS AS IDENTITY` (serial and BY DEFAULT need
  none). MySQL, MariaDB and SQLite write an ordinary column.
- **Scope:** Model, Models, Values, ValuesRows, Unnest, chunked bulk, Upsert and
  OnConflict. A column that is not identity, serial or auto-increment, or not in
  the model, is `ErrValidation`; so are `FromSelect` and `Copy` with Explicit.
- **Zero value:** per dialect. PostgreSQL and SQLite store 0 as written
  because the caller opted in. MySQL and MariaDB refuse a zero in an explicit
  AUTO_INCREMENT column with `ErrValidation`, because the server would generate
  a value instead of storing 0; every row is checked before the first chunk.
- **Sequence:** only PostgreSQL needs `setval`, which the caller runs; Momo
  never calls it. MySQL, MariaDB and SQLite move past a larger explicit value
  on their own.
- **Checks:** a `ColumnRef` of another table, and the same column in both
  `Explicit` and `Omit`, are `ErrValidation`.
- **Upsert:** the explicit column is written by the INSERT only; a serial or
  identity column that is not the conflict key is never updated on conflict.

## Parity: --from-db on SQLite (2026-11-14, integrator)

- **Scope:** `momo generate --from-db` reads a SQLite database. The dialect
  comes from `momo.yaml`; the value is a path, a `sqlite:` or `file:` URL, or an
  environment variable name. The CLI opens it with `driver/sqlitepure`, so the
  binary needs no cgo. The open is read-only (`sqlitepure.Config.ReadOnly`: no
  WAL pragma, `mode=ro`, `PRAGMA query_only`), so `--from-db` and `--check`
  never convert a rollback-journal file to WAL. A database already in WAL mode
  needs its `-shm` and `-wal` files or a writable directory; no `immutable=1`
  fallback, which would be unsafe on a live database. The file must exist,
  because opening a missing one would create it, and `file:` URIs are parsed
  for their path; `:memory:` and `mode=memory` are refused as empty.
- **Reuse:** the inspection is `migrate/sqlite`'s, through `Engine.Inspect`, not
  a second reader. `fromdb.Generate` takes `Options.Dialect` (PostgreSQL,
  MySQL or SQLite). The CLI parses the value with `sqliteTarget`, the parser
  of `momo migrate`, and opens the file through `openFromDBEngine`, which for
  SQLite builds a read-only engine instead of `openDriver`'s read-write one.
  `--schema` stays PostgreSQL only and is refused on SQLite.
- **Inspection fix:** a rowid alias (`INTEGER PRIMARY KEY`) is reported NOT
  NULL. The pragma says nullable for one that is not declared NOT NULL, but it
  cannot hold NULL, and a model cannot declare a nullable key. Another primary
  key column that allows NULL is written NOT NULL with a warning.
- **Table options:** `AUTOINCREMENT`, `WITHOUT ROWID` and `STRICT` had no model
  declaration, so a database that uses them could not round-trip. They are the
  `autoincrement`, `without_rowid` and `strict` options of the `momo.Model`
  marker tag, a diagnostic on any other dialect.
- **Types:** a declared type maps by the SQLite affinity rules (INT, CHAR, CLOB,
  TEXT, BLOB, REAL, FLOA, DOUB), plus BOOLEAN, DATETIME, TIMESTAMP and DATE. The
  declared spelling is kept with a `type:` tag when the Go type alone would give
  another. NUMERIC affinity by default (`DECIMAL(10,2)`, `JSON`) and a column
  with no type stay unmapped errors: there is no one Go type to guess.
- **dbdefault:** the PostgreSQL rule applies unchanged to SQLite, which reads the
  value back with `RETURNING`: computed defaults are tagged, constant ones only
  with `introspect.dbdefault: all`, where a non-zero `bool` or number becomes a
  pointer. MySQL is still excluded.
- **Indexes:** a unique index with no option is `c.Index(...).Unique()` on
  SQLite. The PostgreSQL spelling `c.Unique(...).Method("btree")` would store a
  method that SQLite inspection never reports.
- **Skipped:** policies, extensions, schemas and functions do not exist on
  SQLite and are never read. A SQLite CHECK of the form `col IN (...)` on a TEXT
  column stays an enum column, which models and the SQLite renderer already
  support.

Decision (2026-11-14, integrator): as above.

## Parity: DELETE ... USING and per-row DEFAULT on SQLite (2026-11-14, integrator)

- **DELETE ... USING:** SQLite renders `DeleteQuery.Using` as
  `DELETE FROM t WHERE EXISTS (SELECT 1 FROM src1, src2 WHERE <where>)`; before
  it failed with `ErrUnsupported` (`delete.go`, `render`). EXISTS, not
  `pk IN (SELECT pk ...)`: it needs no key, so a composite key (row-value `IN`
  would need every column in the target's primary key list), a table without a
  primary key and a `WITHOUT ROWID` table all work with one rule; a target row
  is deleted once however many source rows match, the meaning of USING on
  PostgreSQL; the `Where` predicates, which may reference target and sources,
  move into the subquery unchanged and correlate to the outer target. SQLite
  `RETURNING` reads the deleted table only, so `Returning` target columns work
  and a source column stays `ErrUnsupported`, as for `UpdateQuery.From`. The
  unfiltered-mutation guards ran before and are unchanged. An `AllRows`
  statement with no `Where` renders `EXISTS (SELECT 1 FROM src)`, which deletes
  every row when the source is not empty, as USING does. Review fix: a `Using`
  source named like the target (unaliased) is `ErrValidation` on every
  dialect. In the SQLite subquery every reference would bind to the inner
  table and the delete could remove the whole table; PostgreSQL already
  refuses it, so refusing everywhere keeps the dialects consistent, and an
  aliased self-join (`TableRef.As`) is the supported spelling. A raw `Where`
  fragment or interceptor predicate must qualify its columns: an unqualified
  column both tables have binds to the source on SQLite and is ambiguous on
  PostgreSQL (documented on `Using`).
- **DEFAULT in VALUES:** momo wrote `DEFAULT` in `Models` rows (a zero
  `dbdefault` field next to rows that set it) and in `ValuesRows` (a column
  omitted or `SetDefault` in some rows); SQLite refused both with
  `ErrUnsupported` (`dbDefaultDecision`, `valueRows`). Other dialects are
  unchanged (`DefaultInValues`): `ValuesRows` writes `DEFAULT` inline on
  PostgreSQL, MySQL and MariaDB; `Models` does on PostgreSQL and MariaDB, while
  MySQL has no `RETURNING`, so `dbDefaultDecision` still refuses a zero
  `dbdefault` field there with `ErrUnsupported`.
  On SQLite `plan` now splits the rows into runs of consecutive rows with the
  same set of default columns (`defaultRuns`), each an ordinary statement in
  which a column default in every row leaves the column list, run through the
  existing chunk path: one atomic scope (transaction or savepoint), the rows of
  all runs in `Rows`, one shared clock read, read-back per run. A pinned
  `OnConn` handle cannot open the transaction, so a split insert there fails
  with `ErrUnsupported` before any statement runs. Atomicity is tested with a
  unique violation in the third run on a pooled DB and, on SQLite, inside a
  `Tx` (savepoint), and with `Rows` closed early.
- **Why runs and not groups:** grouping all rows by column set would reorder
  them, and with RETURNING or generated keys the order the caller gave is the
  order they read back. Consecutive runs keep it; the cost is one statement per
  change of pattern (the worst case alternates, N statements). Fallbacks
  considered and rejected: writing the column's default expression (momo does
  not know it) and inserting NULL (changes semantics).
- **Not changed:** `SQL()` renders one statement and still fails with
  `ErrUnsupported` on SQLite for a mix, as do `Unnest` and `Copy`. A WITH
  clause that would repeat across runs is `ErrValidation`, as for chunks.
- **Docs:** `docs/capabilities.md` rows and the ai-guide and migrations notes.
  The generated capabilities block lists dialect flags, and `DefaultInValues`
  stays false on SQLite because the dialect still has no DEFAULT keyword in
  VALUES, so no generated row changes.

Decision (2026-11-14, integrator): as above.

## DistinctOn on SQLite, MySQL and MariaDB (2026-11-14, integrator)

- **Approach:** the three dialects have no `DISTINCT ON`, so `DistinctOn`
  renders a derived table `momo_d` that holds the select list (aliased
  `momo_cN`), the ORDER BY expressions (aliased `momo_oN`) and
  `ROW_NUMBER() OVER (PARTITION BY keys ORDER BY order)` as `momo_rn`, over the
  original FROM, joins, WHERE, GROUP BY and HAVING. The outer query keeps
  `momo_rn = 1`, sorts by the `momo_oN` columns and applies LIMIT and OFFSET.
  PostgreSQL still renders its native clause. `dialect.Dialect.WindowFunctions`
  gates it (SQLite 3.25, MySQL 8.0, MariaDB 10.2).
- **Same validation everywhere:** the ORDER BY prefix rule (an OrderBy whose
  leading items are the DISTINCT ON expressions) is checked on every dialect by
  the code PostgreSQL uses, so a query that builds on one builds on all.
- **No helper column leaks:** the outer select list holds only the user's
  columns, so a scan and a raw read see no `momo_rn`. Each select-list item is
  aliased inside the derived table, so a join with two same-named columns does
  not trip MySQL's duplicate-column check, and the outer query gives each item
  back its column name or `As` alias. An item that is neither (a bare function)
  keeps `momo_cN`; alias it.
- **ORDER BY outside the select list:** every ORDER BY expression is projected
  as a hidden `momo_oN` column of the derived table and the outer ORDER BY
  sorts by it, keeping the direction and the NULLS term (MySQL's `IS NULL` key
  included). An ORDER BY on an `As` alias orders the window by the aliased
  expression, because MySQL does not resolve a select alias there; an alias not
  in the select list is `ErrValidation`.
- **Interactions:** WHERE, joins, GROUP BY, HAVING, soft-delete and the keyset
  `After` predicate apply before the dedupe, as on PostgreSQL; LIMIT and OFFSET
  apply after it, on the outer query. `Count` and `Exists` count the groups, as
  on PostgreSQL, from a form whose outer select list is one constant column
  (`momo_one`) without the ORDER BY: wrapping the real select list would give
  MySQL a derived table with repeated names for a join that selects `p.id` and
  `c.id` (error 1060). NULL keys form one group, as in `DISTINCT ON`.
- **Alias check:** the emulation renders an `As` expression itself, so it calls
  the same name check as `Aliased.AppendSQL`; an empty, malformed or over-long
  alias is `ErrValidation` on every dialect.
- **Unaliased expressions:** an item that is neither a column nor an `As`
  alias is `momo_cN` on the emulated dialects, so a subquery or CTE that reads
  it by its PostgreSQL name must alias it.
- **Refused:** `ForUpdate`, `ForShare` and the other row locks with `DistinctOn`
  return `ErrUnsupported` on SQLite, MySQL and MariaDB: the rows are read in a
  derived table, so a lock would not cover the rows returned, and PostgreSQL
  refuses the same combination (`ErrValidation`).
- **NULL placement:** PostgreSQL sorts NULLs last in an ascending order and
  first in a descending one; the other three do the opposite. Where momo knows
  an `OrderBy` item is nullable (a model column or `NullColumn`) and it has no
  NULLS term, the emulation writes PostgreSQL's placement out, so the tiebreak
  picks the same row. For `ColumnOf` and expressions momo cannot know, so those
  need an explicit `NullsFirst` or `NullsLast`; the docs say so. The rows
  therefore equal PostgreSQL's for a total `OrderBy` with that caveat, not
  unconditionally.
- **Tests:** goldens for the three dialects; integration tests with ties and a
  NULL key on PostgreSQL and SQLite (`testopen.Each`) and on MySQL 8.4, MySQL
  9.7 and MariaDB 11.8 (`testopen.EachMySQL`), against the rows PostgreSQL's
  `DISTINCT ON` returns.

Decision (2026-11-14, integrator): as above.

## Parity: DELETE ... RETURNING on MariaDB (2026-11-14, integrator)

- **Capability:** a new `dialect.Dialect.DeleteReturning` flag, set on the
  MariaDB flavor only, next to `InsertReturning`. `Returning` stays false there
  because UPDATE has no RETURNING on MariaDB. MariaDB supports
  `DELETE ... RETURNING` from 10.0.5 and CI runs 11.8, so no version probe is
  needed (the dialect already assumes 10.5 or later for the insert form).
- **Gate:** `renderWriteReturning` takes a delete flag and accepts the clause
  when `Returning` or, for a delete, `DeleteReturning` holds. MySQL keeps
  `ErrUnsupported`, and the delete message names MariaDB as the alternative.
- **Multi-table delete:** MariaDB rejects RETURNING on a multi-table DELETE, so
  a `Delete` with `Using` and `Returning` is `ErrUnsupported` on the MySQL
  family. A source column in the list was already refused outside PostgreSQL.
- **INSERT ... RETURNING:** already enabled on MariaDB (`InsertReturning`, used
  by the identity read-back), so this change leaves it alone.
- **Tests:** golden SQL per flavor, the Using refusal and the UPDATE refusal as
  unit tests; `testopen.EachMySQL` asserts the returned rows on mariadb118 and
  the refusal, with the rows left in place, on mysql84 and mysql97.

## Parity: advisory session locks on MySQL and MariaDB (2026-11-14, integrator)

- **API:** the existing `advisory` session functions (`Lock`, `TryLock`,
  `Unlock` and their `String` forms) now run on MySQL and MariaDB with
  `GET_LOCK`, `GET_LOCK(name, 0)` and `RELEASE_LOCK`. No new function: callers
  pass the same pinned `db.OnConn(pc)` handle as on PostgreSQL, which keeps the
  lock and its release on one connection. A pooled `*DB` stays `ErrValidation`.
- **Key mapping:** the lock name is `momo:advisory:<db>:<key>`. `GET_LOCK` names
  are global to the server and PostgreSQL advisory locks belong to one database,
  so the name is scoped to the current database like `migrate/mysql` does: `<db>`
  is 8 hex digits of `sha256(DATABASE())` (`nodb` when there is no default
  database), computed by the server inside the statement on each call. `<key>`
  is the decimal `int64` or `s:` plus 32 hex digits of the text key's sha256; at
  most 57 characters against the 64 `GET_LOCK` allows. The key mapping is fixed
  by the client, so every process agrees. `Hash64` has no effect.
- **Errors:** a NULL result is a `*momo.Error` (op `advisory lock`, `ErrBusy`), no
  row is `ErrNotFound`, and a context that ends during `Lock` is `ErrCanceled`
  wrapping the context error. After a `Lock` error the caller `Discard`s the
  pinned connection, because a granted lock may still be attached to it.
- **Blocking wait:** `Lock` repeats `GET_LOCK(name, 1)` until granted or the
  context ends, because a negative timeout waits forever on MySQL and returns
  NULL on MariaDB. A NULL result is an error everywhere.
- **Refused, not emulated:** the transaction-scoped functions and the shared
  functions stay `ErrUnsupported` on MySQL and MariaDB. momo has no
  commit or rollback hook for a `Tx` to release a `GET_LOCK` lock, and
  `GET_LOCK` has no shared mode; emulating either would silently hold a session
  lock past the transaction.
- **Verification:** unit tests on the fake driver for the statements, the key
  mapping, the NULL and 0 results and the refusals; integration tests with
  `testopen.EachMySQL` (MySQL 8.4, MySQL 9.7, MariaDB 11.8) for contention,
  try-lock while held, release, counted holds and the blocking wait.
- **Migration engine:** `migrate/mysql` keeps its own `GET_LOCK` code, which
  maps engine keys and reports the holder for `migrate.ErrLocked`; it is not
  routed through the public package.

Decision (2026-11-14, integrator): as above.

## Parity: BulkUpdate on SQLite, MySQL and MariaDB (2026-11-15, integrator)

- **Finding:** `BulkUpdate` existed and was refused with `ErrUnsupported` outside
  PostgreSQL (gated on `dialect.Merge`, with a single key column and no
  chunking). It is extended, not redesigned.
- **SQL:** PostgreSQL and SQLite render
  `UPDATE t SET c = v.c FROM (SELECT k, c FROM t WHERE false UNION ALL SELECT ?, ? ...) AS v WHERE t.k = v.k`.
  MySQL and MariaDB render
  `UPDATE t JOIN (<same derived table>) AS v ON t.k = v.k SET t.c = v.c`. The
  `UNION ALL` form is kept over `VALUES`/`VALUES ROW(...)` because it works on
  all four servers (MariaDB has no `VALUES ROW`) and the empty first `SELECT`
  gives the rows the table's column types and collation, so a NULL or a string
  key compares correctly. A `UNION` derived table is materialized, so MySQL and
  MariaDB accept it although it reads the table being updated. SQLite
  needs 3.33 for `UPDATE ... FROM`, which `UpdateQuery.From` already relies on.
- **Composite keys:** new `BulkUpdateKeys(q, keys, cols, rows)`; `BulkUpdate`
  calls it with one key. Each row holds the key values first. A NULL key value
  matches nothing; a key column listed twice or also as a value column is
  `ErrValidation`. A key listed twice in the rows applies one of its rows (not
  defined which), as before.
- **updated_at and version:** unchanged from 0.11.0: updated_at is the
  statement clock (one read for all chunks), an explicit updated_at is
  `ErrValidation`, the version column is bumped, OnUpdate defaults and field
  validators run per row.
- **Soft delete:** like `Update`, soft-deleted rows match; the caller adds a
  `Where` on the soft-delete column to leave them out. No implicit filter is
  added, so `BulkUpdate` and `Update` agree.
- **Chunking:** `Exec` sizes a chunk from the dialect's `MaxParams` after
  measuring the fixed parameters (updated_at, OnUpdate values, Where) on a
  one-row statement, and runs the chunks through `runAtomic` (a transaction on
  a `DB`, a savepoint on a `Tx`, `ErrUnsupported` on a pinned `DB`), summing
  `RowsAffected`. A row that cannot fit alone is `ErrLimitExceeded`. `SQL()` and
  `Rows` still render one statement and return `ErrLimitExceeded` over the
  limit, because RETURNING over several statements needs the chunked cursor that
  only the insert path has.
- **RETURNING:** PostgreSQL and SQLite return the updated rows; MySQL and
  MariaDB have no `UPDATE ... RETURNING` and return `ErrUnsupported`.
- **Affected rows:** every dialect counts the rows that match a listed key,
  changed or not. The MySQL driver sets `clientFoundRows`, which makes MySQL
  count matched rows instead of changed rows; a duplicate key counts the target
  row once on all four.
- **Verification:** golden SQL per dialect and flavor as unit tests;
  integration tests with `testopen.EachWithMySQL` (PostgreSQL, SQLite, MySQL 8.4,
  MySQL 9.7, MariaDB 11.8) with `WithMaxParams` set low, for composite keys,
  NULLs, matched-row counts, chunk rollback on a `DB` and a `Tx`, RETURNING,
  updated_at, version and the soft-delete scope; a NULL key; a duplicate key in
  one statement; a chunked list on a pinned `DB`.

Decision (2026-11-15, integrator): as above.

## Parity: MERGE on SQLite, MySQL and MariaDB (2026-11-14, integrator)

- **What exists:** `momo.Merge` takes a `Subquery` source (named `src`) and has
  `On`, one unconditional `WhenMatchedUpdate`, one unconditional
  `WhenNotMatchedInsert` and `Returning`. It has no clause conditions, no
  DELETE action, no DO NOTHING and no WHEN NOT MATCHED BY SOURCE. The emulation
  therefore covers exactly the clause combinations the API can express: update
  only, insert only, update plus insert.
- **Why the naive sequence is wrong:** in a native MERGE every source row is
  matched once against the target as it was before any action. UPDATE then
  INSERT ... WHERE NOT EXISTS re-evaluates `On` after the update, so an update
  of a column `On` reads moves rows in or out of the match (a matched source row
  is inserted again, an unmatched one is skipped). INSERT then UPDATE is wrong
  the other way: the update joins the rows just inserted. The source is also
  evaluated more than once, which is wrong for a volatile source or one that
  reads the target.
- **Design:** one atomic scope (a transaction on a `DB`, a savepoint on a
  `Tx`; a pinned `DB` is `ErrUnsupported`), then: drop stale tables; copy the
  source into the temporary table `momo_merge_src` with a row number; create
  `momo_merge_pairs`, one row per (target row, source row) pair that `On` joins,
  holding the source row number, the target primary key and the new value of
  every assigned column, computed from the target row before any write; with a
  matched clause, a cardinality check on the pairs and the pair count; the
  UPDATE, which assigns from the pairs table only; the INSERT of the source rows
  that have no pair; drop both tables. The match and the source are computed
  once, before any write, so the order of the UPDATE and the INSERT no longer
  matters and every `On` and every assigned column is safe. SQLite renders
  `UPDATE t SET ... FROM pairs AS n WHERE <pk match>`, MySQL and MariaDB the
  multi-table `UPDATE t, pairs AS n SET t.c = n.v0 ... WHERE <pk match>` with
  qualified targets. The tables are dropped in a deferred step, because MySQL
  does not roll a temporary table back.
- **Assignment order:** a MySQL multi-table UPDATE assigns left to right, so
  `SET a = src.x, b = t.a` would read the new `a` where PostgreSQL reads the
  old one. Staging every right-hand side in the pairs table makes the result
  order-independent. `SET col = DEFAULT` cannot be staged and is `ErrUnsupported`.
- **Concurrency (not equivalent, documented):** on MySQL and MariaDB the pairs are
  read with `FOR UPDATE` (the table is created empty first and filled with
  `INSERT ... SELECT`, because MySQL refuses `FOR UPDATE` inside
  `CREATE TABLE ... SELECT` with error 1746), which locks the matched target rows and reads their
  latest committed version under repeatable read and read committed, so the
  update side cannot go stale; a target row committed after that read that
  would have matched is a phantom and counts as unmatched, so the insert writes
  it again or fails on a unique key (PostgreSQL has the same window for a
  concurrent insert). The affected-row count is the pair count read under those
  locks. SQLite has one writer: a deferred transaction that read and then
  writes after another commit fails with a busy error, never stale. No test
  covers concurrent writers, and the docs say so.
- **PostgreSQL error kind:** SQLSTATE 21000 (cardinality violation) now maps to
  `ErrTooManyRows`, so `errors.Is` agrees on every dialect.
- **No ON DUPLICATE KEY UPDATE:** it matches on any unique key, not on `On`, and
  has no way to count inserted plus updated rows like MERGE. The sequence is
  used for every `On`.
- **Cardinality:** a target row that more than one source row matches is an
  error in PostgreSQL ("MERGE command cannot affect row a second time"), raised
  only for a row an action would change. The emulation checks it with a
  `GROUP BY <primary key> HAVING COUNT(*) > 1` over the pairs table, before the
  UPDATE, only when there is a `WhenMatchedUpdate`, and fails with
  `ErrTooManyRows` with nothing written. Without that clause duplicates are
  harmless, as in PostgreSQL. A model without a primary key cannot be checked,
  so `WhenMatchedUpdate` there is `ErrUnsupported`.
- **Affected rows:** the number of (target, source) pairs the UPDATE changes plus
  the rows the INSERT wrote, counted from the snapshot, so it does not depend on
  MySQL reporting changed or found rows. A source row matching several target
  rows updates each of them, as in PostgreSQL.
- **NULL keys:** `t.k = src.k` is unknown for a NULL, so the row is unmatched and
  inserted, as in PostgreSQL.
- **Safe, emulated exactly:** WHEN MATCHED UPDATE alone; WHEN NOT MATCHED INSERT
  alone; both together with any `On` and any assigned column, including an
  update of the column `On` reads, because the match is decided before any
  write; two source rows for one target row when there is no update (no error and
  no action, as in PostgreSQL).
- **Refused:** two source rows for one target row with an update
  (`ErrTooManyRows`, nothing written); WHEN MATCHED UPDATE on a model without a
  primary key; `Returning` and `Rows` (the rows would come from a snapshot); a
  pinned `DB`, which has no transaction; `SQL()`, which has no single text
  (`Statements()` lists the sequence). All but the first are `ErrUnsupported`.
- **Not expressible today:** WHEN MATCHED AND condition, a DELETE action, DO
  NOTHING and WHEN NOT MATCHED BY SOURCE are not in the API. If they are added,
  each needs the same snapshot, matched clauses with overlapping conditions must
  be refused until the first-match order can be kept, and BY SOURCE needs its own
  snapshot flag on the target side.

- **Cost and limits:** the account needs the temporary-table privilege; MySQL
  derives the temporary tables' column types and collations (a literal or
  bind-parameter source can meet a different collation and fail loudly, never
  match wrongly); a source with duplicate column names fails and must be
  aliased; a server with `enforce_gtid_consistency` on refuses the temporary
  table in a transaction when `binlog_format` is `STATEMENT` or `MIXED` (older
  MySQL servers whatever the format); a canceled context or lost connection can
  leave the tables on the pooled connection until its next Merge, which drops
  them first. Each is an error, not a wrong result.
- **Tests:** golden SQL for SQLite and for MySQL and MariaDB, the refusals and the
  one-transaction run as unit tests; `testopen.EachWithMySQL` (PostgreSQL native,
  SQLite, MySQL 8.4, MySQL 9.7, MariaDB 11.8) asserts the same final table and
  count for update plus insert, update only, insert only, NULL keys, an update of
  the `On` column, an assignment that reads another assigned column, bound
  arguments in the source and in an extra `On` predicate, one source row for
  several target rows, duplicate source rows with and without an update, an
  empty source, a unique violation after the update and a composite primary
  key; the goldens assert the bound arguments of each statement; the
  transaction tests (savepoint rollback, two Merges in one Tx, caller rollback,
  `ErrTooManyRows` inside a Tx) run on the emulated dialects.

Decision (2026-11-14, integrator): as above.

## Full-text search design (spec fulltext-search, PR #159)

Ten open questions of `.procoder/specs/fulltext-search.md`, each put to the user with a recommendation and alternatives:

- Q1: a portable layer plus first-class dialect builders (recommended); per-dialect builders only; portable layer only
- Q2: a full-text description on `schema.Index`, the SQLite FTS5 table and triggers derived by the renderer (recommended); a virtual `schema.Table`; `Index.Method` plus `With` pairs only
- Q3 and Q10: a stored generated tsvector column plus GIN by default with an expression index as an option, PostgreSQL 12+ for the declarative feature, config as a literal in the expression (recommended); expression index only; trigger recipe only; PostgreSQL 11 via a generated trigger
- Q4: external-content FTS5 with momo-generated triggers (recommended); standalone table; contentless table; no triggers
- Q5: websearch-style syntax translated and escaped per dialect plus a Raw escape hatch (recommended); raw only; plain only
- Q6: a portable higher-is-better score (bm25 negated), comparable only within one query on one dialect, no thresholds, migrate lint warnings (recommended); no portable ranking; scores normalised to 0..1; lint errors
- Q7 and Q8: BOOLEAN MODE for the portable web-style query with natural language mode and query expansion only through the MySQL builder, and one search declaration per table covering one or more columns (recommended); NATURAL LANGUAGE for the plain query; several declarations per table
- Q9: keep `momo.HybridSearch` PostgreSQL only for now (recommended); extend it to SQLite and MySQL/MariaDB in this feature

Decision (2026-11-15, the user, via AskUserQuestion): Q1 to Q8 and Q10 as recommended. Q9 is NOT the recommendation: hybrid (text plus vector) search is in scope for SQLite and MySQL/MariaDB, as reciprocal rank fusion over the FTS5 or MATCH text leg and the existing cosine vector leg (sqlite-vec on SQLite, VECTOR on MariaDB 11.8). MySQL 9.x has no vector distance function, so hybrid there stays `ErrUnsupported`. The spec records all ten as decisions and is complete.

## Next parity work after 0.13.0, and stale agent worktrees

Which of the remaining design-dependent parity items to start next:

- BulkUpdate: one statement that updates many rows with per-row values (UPDATE ... FROM (VALUES ...) on PostgreSQL and SQLite, a CASE or joined derived table on MySQL)
- MERGE emulation: `Merge` on SQLite, MySQL and MariaDB through upsert plus delete, with the combinations that cannot be emulated refused
- Full-text search: a design spec first (PostgreSQL tsvector, SQLite FTS5, MySQL FULLTEXT) before any code
- None for now

And whether to clean up the ~150 unlocked agent worktrees under momo/.claude/worktrees from earlier sprints:

- Delete those whose branch is merged or gone from GitHub, list the rest
- Leave them

Decision (2026-11-14, the user, via AskUserQuestion): start BulkUpdate, MERGE
emulation and a full-text search design spec (no FTS code until the spec is
approved). Delete the agent worktrees whose branch is merged or gone from
GitHub, and list any with unmerged commits.

## Full-text search plan: spec gaps G1-G5 and the SchemaExpander hook

- G1: a PostgreSQL search config that differs from the index cannot be a migrate lint (lint reads migration SQL); check it at build time with ErrValidation (recommended); add a lint rule anyway
- G2: a declared parser on MariaDB is ErrUnsupported and the parser round trip runs on MySQL only (recommended); assume MariaDB accepts WITH PARSER ngram
- G3: add schema.Table.Engine, set only by the MySQL inspector, so a non-InnoDB FULLTEXT table is ErrValidation (recommended); drop the criterion
- G4: lint warns on every CREATE FULLTEXT INDEX on an existing table (recommended); only the first one
- G5: --from-db writes no field for the generated tsvector column (recommended); emit a read-only field
- SchemaExpander: an optional migrate hook expands derived objects on both sides before diffing, so snapshots keep only the declaration (recommended); store the derived objects in the snapshot

Decision (2026-11-15, the user, via AskUserQuestion): accept all fallbacks and the hook.

## Full-text search Task 1: choices the plan leaves open (2026-11-15, integrator)

- `schema.FullText.Validate` returns plain errors, not `momo.ErrValidation`: package `schema` is standard-library only and cannot import the root package. The generator turns the error into a positioned diagnostic and the dialect expansions of later tasks wrap it in `ErrValidation`.
- The one-declaration-per-table check and `Validate` run in the generator's `addIndex`, before the name-collision check, so a `search` tag and a `c.FullText` call of the same name report "one full-text declaration per table" rather than a name clash.
- The `search` tag form is checked per field (`field <GoName>: search needs a text column, it is <type>`); the call form is checked per column in `applyCompanion`, because a column has no Go field name there.
- The companion reader does not seed the tag-built `<table>_search` name into its index namespace; the builder reports the second declaration.
- Review follow-up (2026-11-15, integrator): until the dialect tasks land, the PostgreSQL, SQLite and MySQL/MariaDB index renderers refuse an index with a full-text declaration with `ErrUnsupported` ("full-text declarations are not rendered yet"). Without it PostgreSQL wrote `USING "fulltext"` (invalid DDL) and MySQL wrote a plain FULLTEXT that ignored the parser. So the declaration is recorded in the model and the snapshot, but migrations refuse it until a later release; docs/upgrading.md says so.
- Review follow-up: `ForDialect` also clears the PostgreSQL-only `Expression` on SQLite and MySQL, or those dialects would diff it forever. `isTextType` compares the base type (length stripped) against an exact set and rejects arrays, so `text[]` and an enum named `text_kind` are not text. The FTS5 `remove_diacritics` option is allowed only on `unicode61`.
- An older momo's `schema.Load` does not reject the new `fulltext` key: it uses plain `json.Unmarshal` (no `DisallowUnknownFields`), so it reads the snapshot and drops the declaration silently.

## Make the MySQL and MariaDB CI jobs required checks on main (2026-11-16, integrator)

The epic audit found that branch protection on main requires only the PostgreSQL integration cells (plus lint, unit, race, compile-fail, govulncheck, windows, macos, title, apidiff-docs). The six `integration-mysql` cells (mysql84, mysql97, mariadb on Go 1.26 and 1.27), `fuzz-smoke`, `bench-module` and `docs` are not required, so a MySQL or MariaDB regression can merge.

- Require the six integration-mysql cells (recommended): a MySQL/MariaDB regression blocks the merge like a PostgreSQL one.
- Require integration-mysql plus fuzz-smoke, bench-module and docs.
- Leave branch protection as it is.

Decision (2026-11-16, the user, via AskUserQuestion): require the six integration-mysql cells. Done the same day through the branch-protection API; main now also requires `integration-mysql (1.26|1.27, mysql84|mysql97|mariadb)`.

## MySQL FLOAT columns read back with six significant digits on argument-less queries (2026-11-16, integrator)

PR #196's live type-mapping test found that on MySQL 8.4, 9.7 and MariaDB 11.8 a `float` (float32) column holding `math.MaxFloat32` reads back as `3.40282e+38` when the query has no arguments: the server formats FLOAT with six significant digits in the text protocol. A query with arguments (binary protocol), `CAST(v AS DOUBLE)` and `v + 0e0` all read the exact value. momo's scan path is not the cause.

- Document it (recommended by the agent): docs/TYPE_MAPPING.md states the text-protocol behaviour, the three servers and the workarounds (use DOUBLE, or a query with an argument); the test asserts it. No runtime cost.
- Always use the binary protocol on MySQL: the driver prepares argument-less queries too, so every read is exact; costs a prepare round trip per such query (or a statement cache).
- Render FLOAT columns as `col + 0e0` in momo-built select lists: exact for typed queries at no round-trip cost; raw SQL keeps the six-digit text format.

Decision (2026-11-16, the user, via AskUserQuestion): document it. docs/TYPE_MAPPING.md states the behaviour and workarounds, and the test pins it.

## Issues #204 to #207 and #167: when to take them (2026-11-16, integrator)

Open: #204 Update opt-in to change a primary key; #205 parallel migration branches conflict on momo.sum and the snapshot chain; #206 additive CHECK value lists; #207 standalone sequences; #167 momotest per-test schema rebuild is slow. #204 and #207 are bounded; #205 and #206 change file formats and the differ and need a design first.

- Start now in parallel (recommended): fix agents for #204 and #207; a design spec for #205 and #206 put to the user before code; #167 after.
- After the milestone close-out: finish the open audit PRs and the backlog PR first, then take the issues.
- Only #204 and #207 now; leave #205, #206 and #167 for a later sprint.

Decision (2026-11-16, the user, via AskUserQuestion): start now in parallel. Fix agents for #204 and #207; a design for #205 and #206 goes to the user before any code; #167 after.

## Design for #205 (parallel migration branches) and #206 (additive CHECK value lists) (2026-11-16, integrator)

Design read against the code. #205 root causes: the `h1:` aggregate line in momo.sum changes with every file; new versions always append at the tail; the chain stores full state per migration, so it allows one child per parent; rebase regenerates only generated files; apply refuses a pending migration older than the newest applied (`ErrOutOfOrder`). #206 root cause: `schema.Check` is an opaque string, so a value change is Drop+Add of the whole constraint, and the declaration is one source line.

Recommended package:

- #205: momo.sum format 2 (one line per file, sorted, no aggregate; a `merge=union` .gitattributes line plus `momo migrate hash` resolve merges; old momo refuses format 2 with ErrNewerFormat; `hash --sum-format 1` downgrades). Chain C1: `parent` is the base a migration was generated from; validate derives state by a 3-way fold in version order, so forks touching different tables pass and a same-table fork stays ErrBranchConflict (resolved by rebase). `rebase --renumber` for pending versions instead of allowing out-of-order apply. Later phases: patch files instead of full snapshots (C2); rebase of hand-written migrations (C4).
- #206: `c.CheckIn(&m.Kind, "a", "b", ...)` rendered one value per line, a structured `In` on schema.Check (no snapshot format bump), set diff with a new AlterCheckIn change; PG keeps NOT VALID + `_no_tx` VALIDATE; SQLite rebuilds and its inspector maps named table-level IN checks to Check{In} instead of Enum; MySQL renders one ALTER and is version-gated. Removing a value is a destructive change needing the destructive opt-in, with a hint and a lint rule. The schema-changing custom migration marker is deferred.

Options:

- Accept the recommended package (recommended).
- Accept it, but allow out-of-order apply of a pending migration disjoint from later applied ones instead of `rebase --renumber`.
- Accept it, but keep momo.sum format 1 (aggregate kept) and ship a git merge driver instead.
- Do #206 only now; leave #205 for a later sprint.

Decision (2026-11-16, the user, via AskUserQuestion): accept the recommended package. #205 phase 1 (momo.sum format 2, the fold chain C1, `rebase --renumber`) and #206 in full; C2, C4 and the schema-changing custom-migration marker later.

## Closing milestones M1 to M4 after 0.15.0 (2026-11-16, integrator)

All four milestones are open on the board; two stories are open (momo-07 Task 8 pgx binary vector codecs, momo-12 Task 5 Ent and Bun adapters), every epic shows spec drift, and the parity and full-text work of 0.12.0 to 0.15.0 has no stories.

- Audit then close (recommended): a read-only audit of every epic against the real code, build or drop the two open stories, record the shipped parity and full-text work, then close what the audit shows done.
- Audit only: report what each epic really delivers and what is missing; close nothing yet.
- Build the two open stories first, audit afterwards.
- Leave the board as it is.

Decision (2026-11-16, the user, via AskUserQuestion): build the two open stories first (momo-07 Task 8 pgx binary vector codecs, momo-12 Task 5 Ent and Bun adapters), then audit the epics.

## The two open stories need a benchmark run on dedicated hardware (2026-11-16, integrator)

Both stories are already built and tested in CI (vector codecs with TestVectorBinaryCodec; Ent and Bun adapters with TestBenchEquivalence). The only unticked criterion of each is a committed `make bench` reference run under bench/results (raw output, environment, pinned versions, vector-encoding.txt). docs/PERFORMANCE.md forbids shared CI runner numbers, and the laptop's docker is broken, so no agent can produce it.

- Reword the two criteria (recommended): the tooling and layout test (`make bench`, bench/scripts/run.sh, TestBenchResultsLayout) is the deliverable; a published reference run becomes a separate story owned by the user, and both stories close.
- Run on a CI runner: add a workflow that runs `make bench` and commits bench/results, changing docs/PERFORMANCE.md to accept runner numbers labelled as such.
- Wait for the user to run `make bench` on dedicated hardware and commit bench/results.

Decision (2026-11-16, the user, via AskUserQuestion): reword and close. The tooling and layout test are the deliverable; a published reference run is a separate story owned by the user.

## Spec criteria reworded to what shipped (2026-11-16, integrator)

The read-only audit of M1 to M4 found criteria whose wording no longer matches the shipped, tested behaviour. The user approved the close-out flow "build the two stories, then audit, then close" (decision "Closing milestones M1 to M4 after 0.15.0"); the rewordings below follow that flow and the earlier decisions named beside each. Each cites test names confirmed in the code.

- S-25: version tags (`@vN`) with declared permissions, enforced by `TestWorkflowsUseVersionTags` (decision "GitHub Actions: version tags or commit SHAs").
- S-49: `TestCodeQLAndGosecRunOnPullRequests`.
- S-34: `TestNewTxRollsBack` and `TestNewTxRefusesCommit` (per-test transactions on PostgreSQL), `TestMomotestSQLAssert`, `TestMomotestFixtures`, `TestMomotestSQLiteEphemeral` (SQLite uses throwaway files).
- S-94: Go migrations are forward-only (rollback refused with ErrNoDown); `TestGoMigrationOrderedWithSQLRunsOnce`, `TestRefuseGoRollback`, `TestRollbackGroup` (decision "Carried plan stories (2026-11-11, the user, via AskUserQuestion)").
- S-19 and S-50: `make bench` writes raw output, environment and pinned versions under bench/results, layout enforced by `TestBenchResultsLayout`; a published reference run is a separate story (decision "The two open stories need a benchmark run on dedicated hardware").
- S-89: the pure-Go adapter's own suite (driver/sqlitepure, internal/sqlshared) runs with CGO_ENABLED=0; the root suite uses the cgo driver.
- S-11: PostgreSQL test names kept; added `TestSQLiteStagedChange`, `TestSQLiteConcurrentApply`, `TestMySQLStagedChange`, `TestMySQLConcurrentApply`, `TestMySQLApplyRefusedWhileLockHeld`.
- S-16: `TestSQLiteVectorParity`, `TestSQLiteANNIndexUnsupported`, `TestSQLiteRenderRefusesVectorIndexMethod`.
- S-23: Go blocks are compiled when complete and parse-checked when fragments (ceiling `maxFragmentBlocks` in internal/docscheck/codeblocks_test.go).
- S-31: `TestTypeMappingMySQL` added for the MySQL column.

## momotest template cloning: default or opt-in (#167, PR #219, 2026-11-16, integrator)

On the template database path the test's tables live in the clone's `public` schema, so a suite that relies on `momotest_*` schema names or `current_schema()` would change, and the path would depend on whether the role has CREATEDB.

- Opt-in for 0.16.0 through `momotest.WithTemplateClone`; the default stays schema per test (recommended).
- Default on, which breaks suites that read the schema name.

Decision (2026-11-16, the user, via AskUserQuestion): opt-in for 0.16.0; the default stays schema-per-test.

## #225 drift repair (2026-11-17, integrator)

Issue #225: the adopt refusal gave only counts, `drift` on a database without history listed every table as unmanaged, and no momo command produced the DDL that brings a live database to the snapshot. Decided, not asked:

- The adopt refusal names every differing object: kind, table, column or constraint, and for a changed object the live and snapshot definitions of the fields that differ. It is now a `*migrate.BaselineMismatchError` that wraps `ErrBaselineMismatch` (errors.Is unchanged), keeps the old first line, lists the first 50 objects and then "and N more", and carries the whole `DriftReport`.
- `drift` on a database with no applied migration compares with the latest state of the migration directory (what `baseline` expects) instead of the empty schema; a directory without a snapshot still compares with the empty schema. `ObjectDiff` gains `Snapshot` and `Live` strings; `Detail` is unchanged.
- `drift --fix-sql` prints the DDL live to snapshot, rendered by the dialect's differ and renderer, ordered as `diff` orders a migration, print only (no apply command). Go: `Engine.DriftRepair` returns a `*Repair`.
- Destructive means every `drop_*` change and every column type change. Without `--allow-destructive` those steps, and the steps that depend on them, are not printed; each is listed on stderr and the exit code is 1. A step the dialect cannot render (an error wrapping `momo.ErrUnsupported`) is listed on stderr as not rendered and the exit code is 1; no wrong SQL is printed. On SQLite a change that needs a table rebuild is withheld while another change to the same table is, because the rebuild would apply it.
- A missing NOT NULL column is added nullable with a hint (the differ's staging), since the table may hold rows; an existing column's SET NOT NULL is emitted (EnforceNotNull), because the repair is what makes the database match.
- Not done: no `repair` command that applies the statements. A caller reviews and runs the script.
- Review round: a create that replaces a withheld drop (same kind, schema, table, name: a changed view, trigger, policy, index or constraint) is withheld with it, so the script never prints an "already exists" create. `drift --version N` and `DriftOptions.Version` / `RepairOptions.Version` compare with the snapshot of exactly version N (the one `baseline --version N` checks), and `DriftReport.SnapshotVersion` names the one compared. A repair is complete only when nothing is withheld, unsupported or uncovered (a report entry no change touches: invalid index, unmanaged extension or schema) and no NOT NULL column was staged nullable; `Unverified` entries do not count, as in `drift`. `--fix-sql` keeps standard output SQL only (`no drift`, notes and reasons go to stderr) and notes that MySQL and MariaDB do not roll DDL back.

## Outbox design (#251)

Issue #251, PR #260: the transactional outbox (`outbox` package) with a relay.

Decision (2026-11-18, the user, via AskUserQuestion): the outbox table is a model that users copy from docs/outbox.md; the package ships no importable model.

Decision (2026-11-18, the user, via AskUserQuestion): a dead event keeps blocking the later events of its key until `Requeue` or a delete.

Decision (2026-11-18, the user, via AskUserQuestion): MySQL and MariaDB are supported in this PR. The claim is a short READ COMMITTED transaction (`SELECT ... FOR UPDATE SKIP LOCKED`, `UPDATE` with the lease, commit) before anything is published, the server version is checked on the first claim (MySQL 8.0, MariaDB 10.6; older is `ErrUnsupported`), and the relay polls because there is no LISTEN.

Integrator notes, decided and not asked:

- Delivery is at least once: an event is published again after a relay crashes between the publish and the mark, or when a publish outlives the lease. `Event.ID` is stable and is the deduplication key.
- Order per key is the id order, and an event is claimed only when no earlier event of its key is unpublished. PostgreSQL assigns the id at insert, not at commit, so writers of one key must be serialized by the application (documented in docs/outbox.md).
- Lease and backoff use the clock of the process that claims, so relay hosts need agreeing clocks.
- The table has two plain indexes instead of partial ones, because MySQL and MariaDB have no partial indexes.
- Times are bound as UTC `time.Time` on PostgreSQL and as fixed-width UTC text on SQLite, MySQL and MariaDB.
- `Event.Attempt` is the number of the attempt that is starting, 1 for the first, on every database. PostgreSQL and SQLite return the row from the claiming UPDATE; the MySQL claim selects before it leases, so the relay adds the counted attempt to the selected value.
- The MySQL and MariaDB `payload` column stays `blob` (65,535 bytes): momo maps `[]byte` to `blob` there and the model's `type:` tag is one string for every database, so `longblob` would break PostgreSQL and SQLite. docs/outbox.md states the limit and gives the `ALTER TABLE ... MODIFY payload longblob` migration for larger events. Changing the global `[]byte` mapping on MySQL was not done: it is outside this PR.
- Review round (the outcome fence): published, retry, dead and release match `attempts` as the claim counted it, and retry and dead also require the row pending, so a relay whose lease ended cannot overwrite the relay that holds the event. Zero rows affected is reported through `WithOnError` as an error wrapping the new `outbox.ErrLeaseLost` and sets `Outcome.LeaseLost` (Dead is then false). The stale event may be published twice, which at-least-once already allows. A `Requeue` resets attempts to 0, so a stale outcome of attempt 1 could match a requeued event that was claimed once more; requeue only touches dead events, so this was accepted.
- A claim counts the attempt before the publish, so a crash during a publish consumes it. A claim that counts more attempts than the maximum marks the event dead without publishing it (last_error says the last attempt never ended) and reports it, so a poison event that kills the process stops after the maximum. A publish that outlives its lease on the last allowed attempt can leave a delivered event dead; documented.
- Retry delays get 20 percent jitter either way, capped at the maximum, from an injectable random source (`export_test.go`), so tests stay deterministic. Unparsable headers and publisher panics (with the stack) go to `WithOnError`.
- The Cleanup subselect has no ORDER BY on PostgreSQL and SQLite (the order only decides which rows a chunk takes); MySQL keeps it, where `DELETE ... ORDER BY ... LIMIT` needs it for the oldest-first chunk.

## Embedding sync design (#250)

PR #257 keeps embeddings in sync with their source columns (`embed_from`). Four design points were put to the maintainer.

- Concurrency: compare-and-set on the source columns and stored hash, not `FOR UPDATE SKIP LOCKED`, so no transaction is held during the Embedder call; the cost is duplicate provider calls between runners.
- Staleness: decided in Go from `vector.SourceHash`, not in SQL, because a portable SQL hash is not possible across dialects; the cost is reading every live row's source text per run.
- Write path: a raw write of the vector and hash only (no `updated_at`, version bump or hooks), but the stale read and the write-back both run the handle's interceptors, so tenant scoping applies.
- Hash column name: fixed `<column>_hash`, not configurable.

Decision (2026-11-18, the user, via AskUserQuestion): keep compare-and-set; no change.

Decision (2026-11-18, the user, via AskUserQuestion): keep deciding staleness in Go; no change.

Decision (2026-11-18, the user, via AskUserQuestion): change the write path so the read and the write-back apply the model's interceptors; keep the raw write without hooks, `updated_at` or version.

Decision (2026-11-18, the user, via AskUserQuestion): keep the fixed `<column>_hash` name; no change.

## Range types and exclusion constraints (#253)

Items 1 and 2 of issue #253 (PR #258). Decided, not asked:

- Text codec only: `types.Range[T]` writes and reads the PostgreSQL range text through `Value` and `Scan`. There is no pgx binary codec, because pgx already turns the binary form into text for an `sql.Scanner` and sends a string as text, so the pgx driver and `database/sql` both work without registering anything.
- No multiranges: they would need a second generic type and more predicates. `--from-db` reports a multirange column as unmapped, as before.
- Exclusion constraints model plain columns only (column, operator, access method, deferral flags). The PostgreSQL inspector refuses a constraint with a `WHERE` predicate, an expression, an operator class, an ordering, `INCLUDE` or storage parameters, with an error naming it and its table, instead of skipping it; `migrate.exclude` hides the table. Partial exclusion constraints are the likely follow-up and need the canonicalization round trip.
- `types.Timestamp` is the element of a `tsrange`, so the Go type says whether a column has a time zone and `rangeop` can cast an operand correctly. It has `Infinity` and `NegInfinity` flags like `types.Date`; `time.Time` cannot hold infinity, so a `tstzrange` infinity bound does not scan.
- The zero `types.Range[T]{}` is the empty range: it is written as `empty` and reads back with `Empty` set. Compare with `IsEmpty`, not `==`.
- Range predicates live in a new feature package `rangeop` (listed in `internal/repocheck/testdata/packages.txt` and in the docscheck API list), not in the root package.
- Bare-name allow-list: the docs already name the `Range` window frame and the `Exclude` glob list, which now share a name with `types.Range` and `schema.Exclude`, so `internal/docscheck/testdata/bare_allow.txt` allows them on the pages that use them instead of rewriting those pages.
- MySQL and MariaDB get no integration coverage: they refuse a range field and an exclusion constraint with `momo.ErrUnsupported`, which the render and generator tests cover.
- An exclusion constraint is backed by an index of its own name, so `splitNoTx` treats `add_exclude` and `drop_exclude` like the other index-name changes and moves them into the no-transaction tail after a concurrent drop of the same name.

## Compatibility policy (#254)

Question: which packages are excluded from the v1 compatibility promise, and what happens to the API-review findings?

Decision (2026-11-19, the user, via AskUserQuestion): accept the proposed experimental list: `gen/graphql`, `gen/grpc`, `gen/openapi`, `gen/apiquery`, `importer`.

Decision (2026-11-19, the user): carry out all seven API-review cleanup groups as breaking changes before v1, tracked as separate issues.

## Audit history CI fixes (#252, PR #259)

- The audit history API lives in `db_history.go` (root file prefix `db`), with tests in `db_history_unit_test.go` and `db_history_integration_test.go`; the prefix guard is unchanged.
- The "Audit history tables" feature names only `schema.History` and `schema.HistoryFor`. `checkFeatureAPI` resolves `<package>.<Name>` against a package directory, and the root package has none, so `momo.SetActor`, `momo.HistoryOf`, `momo.HistoryAsOf` and `momo.HistoryEntry` stay in the feature's prose cells only.
- `TestDiffCreatesTheHistoryObjectsInDependencyOrder` expected three changes; Diff also creates the history table's lookup index (`notes_history_row_idx`), which is correct. The test now expects four changes and checks that the table comes before its index and trigger. The empty field in `create_function  notes_history_fn` is the empty table of a function change, not a bug.
- `docs/history.md` names the declaration as `Config.History`, the method the model calls.

## Audit history review round (#252, PR #259)

Decision (2026-11-19, the user, via the coordinator): four design questions of the fresh review.

- Primary key change: an UPDATE whose primary key differs also records the old row as a `DELETE` before the new row, so the old key's timeline closes (`schema/history.go`). A no-op UPDATE records nothing; the rows are compared as text (`OLD::text IS NOT DISTINCT FROM NEW::text`), because a record comparison fails on `json`, `xml` and `point` columns and a `jsonb` comparison treats numeric 1.0 and 1.00 as equal and loses that change.
- Review follow-up: every history column copied from the model is nullable with no default (`schema.HistoryFor`), so a later NOT NULL or default on the model never fails on old rows or backfills a value that was never recorded; `migrate diff` therefore never emits either on a history column. The trigger references its function the way a regprocedure prints it (bare when all-lowercase, quoted otherwise), so a mixed-case or hyphenated table name works and inspection still reads the same text back. `TRUNCATE` stays unrecorded and is documented.
- Renames follow: `migrate.Diff` repeats `--rename-table` and `--rename-column` of a model on its history table (`migrate/diff_history.go`). The history function, trigger and row index are rebuilt under the new names (drop and create, no data in them); the old trigger is dropped on the renamed table. No rename of the index in place, since the plan has no rename-index change.
- Dropped model column: the column stays on the history table, relaxed to nullable, so the audit data survives. `adaptHistory` adds it to the desired schema, so the migration snapshot records it and drift sees no difference; removing it is a manual migration. `generate --from-db` does not recognise a history with such a column and models it as ordinary objects (documented).
- Tenant scope: the models have no tenant column of their own; tenancy is the explicit `TenantFilter(ctx, column)` of a `Where`. `HistoryOf` and `HistoryAsOf` therefore take Predicate values among their trailing arguments, aliasing the history table as the model's table so model column descriptors resolve. A missing or mistyped tenant fails with `ErrValidation` through the predicate itself. The read does not pick a tenant up from the context on its own.
- `HistoryOf` is ordered by `history_id`; `HistoryAsOf` stays by `history_changed_at` (trigger time, not commit time) and the docs say so. A soft delete is an UPDATE.
- The feature entry cannot list the `momo.*` names: `checkFeatureAPI` resolves `<package>.<Name>` against a package directory and the root package has none, so they stay in the prose cells and the guard is unchanged.

## Driver constructors (#268)

Question A: who closes the pool behind a driver made by an `Open` function, and how does an application do it?

- `momo.DB.Close` closes the driver when it implements `io.Closer` (the driver an `Open` returns) and returns nil otherwise; a pool wrapped by a `New` is never closed by momo.
- A bare `io.Closer` assertion on the driver in every example.

Decision (2026-11-19, the user, via AskUserQuestion): add `func (db *DB) Close() error` to `momo.DB`, with the first behavior. Docs, README and examples use `defer db.Close()`.

Question B: does `pgxdriver.Open` ping?

- Ping on `ctx` like the SQLite and MySQL `Open`; on failure close the pool and return the error.
- Stay lazy like `pgxpool` and `NewPool`.

Decision (2026-11-19, the user, via AskUserQuestion): `pgxdriver.Open` and `OpenSQL` ping by default; `NewPool` stays lazy.

## Exported internals of schema, migrate and gen (#271)

Question: how do the exported internals of `schema`, `migrate`, `migrate/sqlite` and `gen` leave the public API, and what does each leave behind?

Decisions (2026-11-20, the maintainer, in the brief of #271):

- `momo.NewOffline` is renamed `momo.MustNewOffline(d dialect.Name) *DB` and keeps its panic for an unknown name; `NewOffline` stays one release as a deprecated forwarder.
- `dialect.For` returns a fresh copy on every call, copies of `Vector.Metrics` and `Vector.Indexes` included, made with the plainest copy (`slices.Clone`); there is no accessor API.
- `momo.SetConfigLocal` is deprecated this release in favor of `Tx.SetLocal` and not removed.
- `MySQLVectorType`, `SQLiteVectorType` and `SQLiteBitsType` leave the root package for schema-side internals.
- A moved name keeps a deprecated forwarder for one release only when that is cheap; a forwarder that would keep an internal type exported is dropped and listed in `docs/upgrading.md`.
- `schema.SchemaNamespace` becomes `schema.Namespace` and the old name stays as a deprecated alias.
- `migrate.MatchObject` was a one-line forwarder of `schema.MatchObject`; it is removed.

Decisions made while implementing (2026-11-20, the implementer), with the reason:

- An internal package cannot import a public package that imports it, so a helper that the public package itself calls and that takes the public package's types cannot move into `internal/`. Where that applies the name is either unexported in place (no caller outside the package, or only tests that become in-package tests) or, when another package outside it still needs it, kept exported.
- The nine names `migrate.RunMigration`, `RunEnv`, `RunStatements`, `LintSQL`, `Order`, `OrderErr`, `OrderWithViews`, `EnsureHistory` and `ReadHistory` stay exported. The Engine calls all of them and they take `migrate.Change`, `Statement`, `Dialect` or `HistoryRow`; moving them means moving those types behind an exported alias, the pattern the API review criticizes for `sqlitedriver.Config`. The dialect packages and their tests call them, and the `Render` contract of `migrate.Dialect` tells its caller to order changes with `OrderErr` or `OrderWithViews`, so whoever renders a plan needs them. They are listed in `docs/upgrading.md` as kept.
- `gen.Header` stays exported although the issue lists it: `gen.Writer` refuses plugin output that lacks the header, so a plugin author needs it. `gen.DescriptorIdents` stays too: its doc comment promises that generated code which refers to a descriptor uses these names, and `gen/graphql`, `gen/grpc` and `gen/openapi` do. `gen.HeaderPrefix` and `gen.ParseHeader` move.
- `gen/tags` keeps the `Tags` struct because it is the type of `gen.Field.Tags`; the parser, `Problem` and `SplitOptions` move to `internal/tagparse`. `gen/typemap` moves whole because `Map` is its only export. `naming.ToSnake` is removed (it forwarded `internal/snake`), `naming.Plural` moves to `internal/pluralize`.
- `ErrInvalidVectorIndex` stays in `schema`: the validation functions that wrap it move, but callers match the error.
- The vector column type helpers keep their behavior and move to `internal/vectortype` as `MySQL`, `SQLite` and `SQLiteBits`; the package imports `momo` for `momo.Error`, so it is an internal-fixture, not an internal-leaf.
- `schema` keeps an unexported `sameCheck` beside `internal/schemaops.SameCheck`: `Equal` needs it and the internal package cannot be imported back. Both are a few lines over `Check.ValueList`.
- Review round (2026-11-20, the coordinator): every moved name whose forwarder needs no internal type in its signature and no import cycle keeps a one-line deprecated forwarder for one release: the `schema` helpers except the vector index validators, `migrate.RedactURLs`, `SplitStatements`, `IsNoSuchTable` and exported wrappers over the unexported `ParseDirectives`, `Discover`, `NextVersion`, `Chain` and `RefuseGoRollback`, the `gen` identifier, header and emitter helpers, and `naming.ToSnake` and `Plural`. They have no forwarder, for a cycle: the `schemaops` validators, `tags.Parse`, `Problem` and `SplitOptions`, the `vectortype` functions, and the SQLite `Capture`, `Dependent`, `Rebuild` and `RebuildStatements`. `typemap.Map` has none either, because a forwarding package would keep the import path public. Each forwarder has a table test against its target.
- The `migrate` runner functions (`RunMigration`, `RunStatements`, `RunEnv`, `LintSQL`, `Order*`, `EnsureHistory`, `ReadHistory`) stay exported because of the type cycle; this deviates from #271 and `docs/upgrading.md` says so.
- The directive names the splitter knows are one exported list, `sqlsplit.KnownDirectives`, and a `migrate` test feeds each name to `parseDirectives`. A test compares `schemaops.SameCheck` with `schema.SameCheck` (the copy `Equal` uses). `internal/redact` compiles its expressions on first use with `sync.OnceValue`, because the root package imports it.
- `SplitStatements` moves to `internal/sqlsplit`; its `-- momo:split` header scan is its own copy of the part of the directive parser that reads that one directive, so the splitter has no dependency on `migrate`.

## Colliding and inconsistent API names (#272)

Decision (2026-11-20, the maintainer, in the task brief): rename the names the issue lists, keep each old name one release as a deprecated alias or forwarder where Go allows it, and do not rename the raw SQL family yet.

Choices made while carrying it out (the issue left the exact names open):

- `migrate.Config` becomes `migrate.Settings` (`Config` is a type alias). The option that takes it becomes `migrate.WithSettings`, so the With* rule of #266 holds; `WithConfig` forwards. `gen.Config` becomes `gen.LoadConfig` (alias kept). `migrate.ConfigFromFile` and `gen/config.File` keep their names: they read a file.
- `migrate.FormatVersion` becomes `migrate.HistoryFormat` (constant forwarder). `schema.FormatVersion` and `gen.FormatVersion` stay: each versions a file format, and the suffix rule the review wants ("`*Format` for file formats") is met by the history table being the one name that hid its artifact.
- Date functions: `sqlfn.CurrentDate`, `Infinity`, `NegInfinity` move to `timeop` and return `timeop.Expr` (the existing time-valued expression), so no second date type is added. A feature package may import only the core packages (`TestFeaturePackagesImportOnlyAllowed`), so `sqlfn` cannot forward to `timeop`: the deprecated `sqlfn.CurrentDate`, `Infinity`, `NegInfinity` and `DateExpr` keep their own implementation for one release. `timeop.DateExpr` (the `types.Date` expression) is untouched.
- fts: all PostgreSQL-only names get the TS prefix, not only the four the issue names: `TSVector`, `TSQuery`, `TSWebsearchQuery`, `TSPlainQuery`, `TSPhraseQuery`, `TSMatch`, `TSRank`, `TSRankCD`, `TSHeadline`. Leaving `ToVector` and `Headline` bare would keep the prefix rule half applied. `Search`, `Score`, `Snippet`, `FTS5Match`, `BM25`, `MySQLMatch`, `MySQLRelevance` stay.
- Coalesce: both stay. `momo.Coalesce(e, fallback)` takes a bound value and `sqlfn.CoalesceExpr(a, b, more...)` takes expressions, which is the X / XExpr pair. Dropping the sqlfn form would leave no n-ary expression form, and the root form cannot move (#270 is shrinking the root package). `momo.Func` (any scalar function) and `agg.Func` (an aggregate call) stay and are documented as a pair.
- `sqlfn.NullIf` takes `T any`, like `NullIfExpr`, so the value form now also accepts strings, booleans and dates; widening is compatible and narrowing `NullIfExpr` would have broken string callers.
- X / XExpr rule: X takes Go values, XExpr takes expressions for the same operands. `sqlfn.SubstrFrom` is not an expression form, so it becomes `SubstrToEnd` and `SubstrExpr` is added (positions are `Expression[int32]`, since PostgreSQL's `substr` takes integers); `timeop.IntervalExpr` becomes `IntervalOfExpr` (value form `IntervalOf`). The other packages were read for the same pairs and follow the rule already (`Add`/`AddExpr`, `AddDays`/`AddDaysExpr`, `AtTimeZone`/`AtTimeZoneExpr`).
- `momotest.New` and `sqlitetest.New` take `momotest.Option`; `sqlitetest` honors `WithMomoOptions` and ignores the PostgreSQL-only options. `momotest.MomoOptionsOf` is the exported reader. `New` cannot keep an old form, since a Go function has one signature. `momotest.Insert` becomes `Rows` and `momotest.Graph` becomes `GraphOf` (forwarders kept).
- Marker types: no change. `momo.Model`, `momo.View` and `momo.Projection` are embedded zero-sized structs and `momo.Config` is the receiver of `MomoConfig`; the collision was `migrate.Config` and `gen.Config`, now renamed. `momo.Config` stays because user models and generated code name it.

### Raw SQL family renames (approved)

Decision (2026-11-20, the maintainer, via the coordinator): approved exactly as proposed. `Scalars` becomes `QueryScalars`, `RawRows` becomes `QueryCursor`, `RawSubquery` becomes `SubquerySQL` and `RawQuery` becomes `CompilableSQL`; `Exec`, `Query`, `QueryRows`, `SQL` and `TypedSQL` stay. The old names are deprecated forwarders for one release, and `momovet` checks both. It was a proposal (pending) until then; the docs list the family in `docs/ai-guide.md`, "The raw SQL family".

Rule: a function that runs hand-written SQL is a verb and takes no `Raw` prefix (`Exec`, `Query`, `QueryRows`, `QueryScalars`, `QueryCursor`). A function that wraps hand-written SQL for a builder ends in `SQL` and names what it makes (`SQL`, `TypedSQL`, `SubquerySQL`, `CompilableSQL`). `Raw` is in no name, because every function here is raw.

| Old           | New             | Why                                                                                  |
| ------------- | --------------- | ------------------------------------------------------------------------------------ |
| `Exec`        | `Exec`          | already the verb for a statement that returns a count                                |
| `Query`       | `Query`         | already the verb for typed rows                                                      |
| `QueryRows`   | `QueryRows`     | already the streaming form of `Query`                                                |
| `Scalars`     | `QueryScalars`  | joins the `Query*` verbs: it is `Query` for one column                               |
| `RawRows`     | `QueryCursor`   | the open-cursor form beside `QueryRows`; `RawRows` looked like a twin of `QueryRows` |
| `SQL`         | `SQL`           | already the predicate fragment                                                       |
| `TypedSQL`    | `TypedSQL`      | already the typed fragment                                                           |
| `RawSubquery` | `SubquerySQL`   | fragment that ends in `SQL`, no `Raw`                                                |
| `RawQuery`    | `CompilableSQL` | `RawQuery` next to `Query` suggests it runs; it only wraps                           |

## Expression extension surface (#270)

Decision (2026-11-19, the maintainer, on #254 items 1.7 and 1.8): third-party typed expressions are supported, with a small frozen surface; the rest moves behind an internal bridge; `momo.ColumnFlags` types the column flags.

Frozen, each justified in `docs/compatibility.md`: `momo.Expr`, `momo.Expression`, `momo.Ops`, `momo.NewOps`, `momo.Typed`, `Builder.Ident`, `Builder.Arg`, `Builder.Dialect`, `Builder.SetErr`, plus two the issue allowed "if a minimal external expression provably needs them":

- `Builder.WriteString`: without it an expression can write only identifiers and parameters, never a function call or an operator.
- `momo.Flagged` (with its result type `momo.ColumnFlags`): a wrapper must forward the wrapped column's flags, or `Ops` comparisons stop masking values compared with a sensitive column. An external type reads the flags by asserting `Flagged` itself, so `FlagsOf` can leave.
- `Builder.QualifiedIdent`, `Builder.String`, `Builder.Args`, `Builder.Err` are not in the issue's list and are not frozen or moved here; they are not extension points and stay as they are.

Moved: `WriteByte`, `MarkSensitive`, `ArgFor`, `RenderSensitive`, `ExprFor`, `AppendFilter`, `Unsupported`, `NewBuilder`, `PredicateFunc`, `FlagsOf`, `IsBound`, `ValidName`, `ValidTypeName`, `WindowOf`, `VectorColumnOf`, `MetaTableSchema`.

Design choices, taken without a question to the user because each is the simplest pattern that satisfies the issue:

- Bridge pattern: `internal/bridge` is a leaf (stdlib and `dialect` only, role internal-leaf, added to the core's allowed imports) that holds a `Kit` of function values typed with `any`; the root fills it from an `init` in `expr_bridge.go`; `internal/exprkit` (role internal-fixture, imports the root) wraps the `Kit` in typed functions that the feature packages call. A package that the root imports cannot import the root, so the registry cannot be typed; the two-package split keeps the types in `exprkit`. No exported root name leads to the registry.
- Generic names cannot cross a registry of function values. `FlagsOf`, `PredicateFunc` and `MetaTableSchema` need no root state (`exprkit.FlagsOf` asserts `Flagged`, `exprkit.Predicate` is `momo.Typed[bool]` around an adapter, `exprkit.TableSchema` reads `Meta.TableSchema`); `VectorColumnOf` is replaced in `vecop` by an interface assertion on the four accessors every vector column has; `WindowOf[T]` is replaced by `exprkit.WindowCall`, which returns the erased window expression, and `agg` gets its own `agg.WindowExpr[T]` (Ops, As, AppendSQL) as the result of `Call.Over`. That changes the result type of `agg.Call.Over` from `momo.WindowExpr[T]`; it is listed in upgrading.md.
- Deprecated forwarders: every moved name stays on the root in `expr_deprecated.go` with its old signature (flags as `uint16`) for one release, tested in `expr_deprecated_test.go`. They cannot keep one thing: a type that implements the old `Flags() uint16` no longer satisfies `Flagged`, because `Flagged` now returns `ColumnFlags`; upgrading.md says so. `ColumnMeta.Flags`, the `New*Column` parameters and the `Flag*` constants change type; generated code passes constants or `0`, so the emitted text is unchanged and the generator needs no edit.
- The limit that follows from freezing `Arg` without `ArgFor`: a value an external expression binds with `Arg` itself is not masked when it is compared with a sensitive column; comparisons made through the `Ops` methods are masked. It is documented in `docs/compatibility.md` and in the `Ops` doc comment.
- The worked example is `ExampleNewOps` in `expr_extension_example_test.go` (package `momo_test`), compiled and run by the root test job.

Review round (coordinator, #280), decisions taken as directed:

- Fail-open masking: a wrapper that still implements `Flags() uint16` compiles after the retype, so `flagsOf` (root) and `exprkit.FlagsOf` also accept the legacy `interface{ Flags() uint16 }` and convert it, and keyset pagination reads the flags through `flagsOf`. The path is marked deprecated and is removed in the next release. A wrapper that renders the sensitive column itself is still masked by the render-time detection, so the exposed case is a wrapper that writes the column name with `Builder.Ident` and declares sensitivity only through `Flags`; the test uses that shape. Proven red first on `refactor/270-expression-extension-surface-red` with `TestLegacyUint16FlagsWrapperStaysMasked`, then that branch was deleted. This replaces the earlier statement that a legacy `Flags() uint16` type stops being masked.
- The example no longer binds a caller value with a raw `Arg` beside a column: the comparison goes through an Ops method (`char_length(...).Gt`), the Typed predicate binds nothing, and the file header says `Arg` does not mask. `Builder.Arg` is unchanged.
- `internal/bridge.Register` panics naming the first nil `Kit` field; `internal/exprkit/exprkit_test.go` calls every function once.
- `Order.AppendOrder` moved behind the bridge (`exprkit.AppendOrder`, used by `agg`) and stays as a deprecated forwarder for one release, listed in the upgrading table. This replaces the earlier choice to keep it exported as non-extension API.

## Foreign key ordered before its unique index (#248)

Decision (2026-11-20, the implementing agent, no question to the user): `migrate.Order` moves a foreign key out of a `create_table` into an `add_fk` when the columns it references become unique only through a `create_index` (unique, plain, not partial), `add_unique` or `add_primary_key` of the same plan. `add_fk` already runs after all three.

- Why not "order the unique index ahead of the table": for a table created in the same plan the index cannot exist before its table, so the only fix there is to split the foreign key out. Why not a dependency edge: the orderer is phase based, and `sortTables` only sorts create_table against create_table; the dependency is between a table's inline constraint and a change in a later phase, which an edge between changes cannot express without reordering the phases. The existing cycle breaker already splits a foreign key into an `add_fk`, so this reuses that mechanism and every renderer already handles an `add_fk` for a created table (SQLite folds it back into the `CREATE TABLE`).
- Only a foreign key whose referenced column set equals a key made by the plan moves, compared as a set (PostgreSQL accepts any column order). A foreign key to a primary key or constraint written in the `create_table`, or already in the database, stays inline, so the DDL of existing schemas does not change.
- Drops need no change: `drop_fk` is a phase before `drop_index`, `drop_unique` and `drop_primary_key`. A test now pins that, and the replaced-index case.
- Dialects: PostgreSQL (SQLSTATE 42830) and MySQL/MariaDB (the referenced columns need an index) are fixed. SQLite does not check at DDL time and its renderer folds the `add_fk` back into the `CREATE TABLE`, so its output is unchanged.
- Red first on `worktree-agent-a2e18e87a31e9d62a-red` (run 44768687067), then that branch was deleted.
