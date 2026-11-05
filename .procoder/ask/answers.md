# What a human decided

Written 2026-11-05 04:51 UTC. procoder reads this
file to avoid asking a question twice; edit an answer here to change what
it believes. Reword the question and it will be asked again.

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
