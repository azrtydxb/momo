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
