# What a human decided

Written 2026-11-06 10:05 UTC. procoder reads this
file to avoid asking a question twice; edit an answer here to change what
it believes. Reword the question and it will be asked again.

## [decision] decisions.md

Key: 13214d8ab382
Question: Where story verification runs

- Push each story branch (and sprint-2) to origin so GitHub Actions runs the full suite, race, lint and the pg16/pg17 matrix; agents run only targeted tests locally (recommended)
- Keep everything local, but cap concurrent agents at two and move GOCACHE off the full DATA disk
- Keep the current setup

Decision (2026-11-06): push branches, use CI; standing approval to push worktree-agent-* story branches and sprint-2 to origin (no PRs or merges to main without asking); local runs limited to the story's own tests.

Answer: Push branches, use CI (user, AskUserQuestion 2026-11-06): standing approval to push worktree-agent-* story branches and sprint-2; no PRs or merges to main without asking; local runs limited to the story's own tests.

## [decision] decisions.md

Key: a8fe0cf11cac
Question: Recreate the momo PgBouncer test container with the new pgbouncer.ini (story 065)

- Recreate now: `docker compose -f internal/testdb/compose.yaml up -d --force-recreate pgbouncer`, then re-run the PgBouncer integration tests against the new config (max_prepared_statements = 0, plain auth, userlist).
- Leave the running container as is; the new config is applied the next time the test database is brought up with `make db-up`.

Decision (2026-11-05): recreate now. Mounting the ini failed (Docker Desktop cannot share /Volumes/DATA), so the same settings are applied through the image environment; the container was recreated with them.

Answer: Recreate now (user's choice via AskUserQuestion, 2026-11-05).

## (no longer asked)

Key: after-sprint-1

Answer: Push init to azrtydxb/momo and open a PR into main so CI runs; start sprint 2 (plan 09 Task 1 + plan 02 generator) in parallel.

## (no longer asked)

Key: build-priority

Answer: PostgreSQL and pgvector are built first (M1 end to end), then SQLite+sqlite-vec parity (M2), MySQL/MariaDB (M3), then ecosystem, adoption, performance, docs site (M4).

## (no longer asked)

Key: ci

Answer: GitHub Actions on free GitHub-hosted runners only.

## (no longer asked)

Key: cli-distribution

Answer: go install only; no prebuilt binaries.

## (no longer asked)

Key: column-layout

Answer: One subpackage per model (document.TenantID).

## (no longer asked)

Key: config-format

Answer: YAML momo.yaml (go.yaml.in/yaml/v3).

## [decision] decisions.md

Key: d0b438646f66
Question: Isolate the PgBouncer suite run in a scratch database (story 136 / 065)

The edoburu PgBouncer image cannot track `search_path` (no env variable for `track_extra_parameters`, and mounting an ini fails on this host and in CI), so pooled backends keep stale per-test schemas and `suite.RunAll` through the pooler is red.

- Route every database through the pooler (env `DB_HOST`/`DB_USER`/`DB_PASSWORD` without a database name gives a `* =` entry) in compose and CI, recreate Momo's pgbouncer container once more, and run the pooled suite in a per-run scratch database `momo_bnc_<hex>` that is dropped afterwards.
- Leave the pooler as is and schema-qualify every table name in the shared suite.
- Leave phase 2 red and carry the S-30 criterion of story 065.

Decision (2026-11-05): scratch database per run.

Answer: Scratch database per run (user's choice via AskUserQuestion, 2026-11-05).

## [decision] decisions.md

Key: d2145a7f634b
Question: v0.1.0 scope and release push

- Lock scope: merged relations plus stories 149, 154 and 150; stories 151, 152 and 155 move to 0.2; approve pushing sprint-2 and opening the PR into main once the scope is done (recommended)
- Lock that scope, but ask again before pushing
- Wait for all of momo-06 (stories 150 to 155) before release

Decision (2026-11-06): wait for all relations (stories 150 to 155) before v0.1.0.

Answer: Wait for all relations (user, AskUserQuestion 2026-11-06): stories 150 to 155 before v0.1.0.

## (no longer asked)

Key: docs-ai

Answer: llms.txt + llms-full.txt, raw .md per page, agent usage guide with AGENTS.md snippet, all doc code blocks tested in CI.

## (no longer asked)

Key: docs-tool

Answer: MkDocs Material on GitHub Pages, deployed by GitHub Actions.

## (no longer asked)

Key: docs-versions

Answer: Versioned per release with mike (/latest/, /vX.Y/, /dev/ from main).

## (no longer asked)

Key: ecosystem

Answer: Build first-party GraphQL, gRPC and OpenAPI generators, plus generator plugins and momo viz.

## (no longer asked)

Key: extra-scope

Answer: All 17 suggested gaps added to the spec (S-24..S-40).

## (no longer asked)

Key: go-version

Answer: Go 1.26 minimum; CI on 1.26 and 1.27.

## (no longer asked)

Key: license

Answer: Apache-2.0.

## (no longer asked)

Key: merge-pr-1

Answer: Merge when green via /procoder:merge (squash into main, delete remote init), then rebase sprint-2 onto main.

## (no longer asked)

Key: migration-numbering

Answer: UTC timestamps with linear snapshot-chain check and migrate rebase.

## (no longer asked)

Key: model-shape

Answer: Embedded momo.Model marker + momo tags + statically-read MomoConfig companion method.

## (no longer asked)

Key: module-path

Answer: github.com/azrtydxb/momo; public repo https://github.com/azrtydxb/momo.

## (no longer asked)

Key: mysql

Answer: In scope as a third dialect (adapter, migrations, vector where the server supports it).

## (no longer asked)

Key: mysql-versions

Answer: MySQL 8.4 LTS + MySQL 9.7 LTS + MariaDB 11.8 LTS; no innovation releases; vectors: MariaDB search, MySQL 9.7 storage only, 8.4 none.

## (no longer asked)

Key: next-step-after-backlog

Answer: Commit the spec, the 13 plans, ARCHITECTURE and the backlog on branch init, then start sprint 1 with M1 stories 001–023 (foundation).

## (no longer asked)

Key: orm-gaps

Answer: Add all of groups A-F (S-59..S-95), conflicts only in explicit Momo form.

## (no longer asked)

Key: pgbouncer-container-recreate
Question: Recreate the momo PgBouncer test container with the new pgbouncer.ini (story 065)

Answer: Recreate now (user, 2026-11-05).

## (no longer asked)

Key: pgbouncer-suite-isolation
Question: Isolate the PgBouncer suite run in a scratch database (story 136 / 065)

Answer: Scratch database per run (user, 2026-11-05).

## (no longer asked)

Key: pr-1-merge-strategy

Answer: Merge commit (gh pr merge --merge): all sprint-1 commits stay on main so story evidence hashes stay reachable; sprint-2 merges main with no history rewrite. Supersedes the squash/rebase part of merge-pr-1.

## (no longer asked)

Key: public-author-email

Answer: Keep pascal@watteel.com; push the commits as they are.

## (no longer asked)

Key: purpose

Answer: Both — used in our own products and published as open source.

## (no longer asked)

Key: release-boundary

Answer: v0.1.0 only after all delivery stages 1-9.

## (no longer asked)

Key: release-flow

Answer: release-please from Conventional Commits; CHANGELOG.md generated; tags vX.Y.Z.

## (no longer asked)

Key: secperf-scope

Answer: All 19 suggestions added (S-41..S-58).

## (no longer asked)

Key: sqlite-driver

Answer: mattn/go-sqlite3 (cgo) + sqlite-vec via asg017/sqlite-vec-go-bindings/cgo.

## (no longer asked)

Key: versioning

Answer: Normal Go module SemVer. v0.x no compatibility promise; v1.0.0 when declared stable; breaking changes after v1 require v2 with /v2 path.

## (no longer asked)

Key: windows

Answer: Supported; SQLite/generator suite tested on windows-latest.
