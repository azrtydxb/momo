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
