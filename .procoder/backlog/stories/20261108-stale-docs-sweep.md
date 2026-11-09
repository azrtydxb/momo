# Stale-docs sweep of docs/*.md and README against the real code

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Compare every claim in `README.md`, `llms.txt` and `docs/*.md` with the code on this branch and fix what disagrees, so the later guards start from a clean baseline. Concrete lines found while planning (see section 8 of the research note): `docs/migrations.md` line 986 names `v0.1.0-dev` outside an upgrade context; `docs/vectors.md` lines 20 to 34 present the sqlite-vec `v0.1.6` plan and hybrid-search options `TextCandidates`, `VectorCandidates`, `RRFK` as pending although no such identifier exists in non-test code; `docs/README.md` says the adopt example has 'no model generation from a database in v0.3.0' while `momo generate --from-db` exists (README line 433, `gen/fromdb`); `docs/README.md` points readers to the private planning file `.procoder/specs/momo.md`; `docs/ARCHITECTURE.md` is a plan contract with `[momo-NN]` tags, a 'Planned, not implemented' list naming `momotest`, `momolog`, `otelmomo`, `importer/{gorm,bun,ent}`, `gen/{graphql,grpc,openapi}`, `Engine.Lint`, a roadmap and ownership sections, and a claim that only `cmd/momo` is an 'other package' while `gen/apiquery` and `gen/viz` exist; `docs/supported-versions.md` lists pgx `v5.11.0` against `go.mod` `v5.12.0`; `docs/capabilities.md` says MySQL JSON operators are 'not usable yet' beside rows that give MySQL equivalents; `docs/TYPE_MAPPING.md` lines 73 to 76 carry a status line that will go stale. The 'SQLite and MySQL are not usable' sentence appears by hand in eight places; it is accurate today (`driver/` holds only `pgxdriver`) and is replaced by the generated block in story generated-capability-matrix-and-type-table, not edited here. Split the planning content of `docs/ARCHITECTURE.md` out to `.procoder/` (where plans already live) and leave a reader-facing architecture page that describes packages that exist. Docs-site navigation and page structure belong to momo-13 stories 252 and 254 and are not changed here.

## Acceptance criteria

- [x] each stale line listed in the description is fixed or deliberately kept with a reason in the PR; the PR lists the line, the evidence (file and line of the code that disagrees) and the fix
- [x] a second pass reads every section of `README.md`, `docs/ai-guide.md`, `docs/relations.md`, `docs/projections.md`, `docs/fulltext.md`, `docs/vectors.md`, `docs/migrations.md`, `docs/capabilities.md` and `docs/supported-versions.md` against the code and appends any further stale lines found to the PR with the same evidence format (no section is skipped; the PR names each page as 'checked')
- [x] `docs/ARCHITECTURE.md` no longer contains `[momo-` tags, 'Planned, not implemented', 'Delivery order' or 'Ownership decisions'; a test in `internal/repocheck` fails if `docs/*.md` contain a `[momo-NN]` tag or a link to `.procoder/`
- [x] `docs/README.md` lists every page in `docs/` (a test compares the index links to the directory listing) and no longer names `.procoder/specs/momo.md`
- [x] every example program named in `docs/README.md` exists under `examples/` (existing `TestExamplesAreWired` plus a docs-index check)
- [x] no change to nav structure, mkdocs files or the llms generation (momo-13 stories 252, 254 and 256 own them)

## Evidence

Green: ci-agent run 42498774870, headSha 7e303a02c92551bc758aa300a84b4d3aa373c198, all jobs success. The first run on 48603f3 (42494748386) failed only on TestVectorDocPins, which pinned the removed hybrid-search text; fixed in 7e303a0.

Lines fixed (line, code evidence, fix):

- migrations.md `v0.1.0-dev`: gen/header.go writes a header with only the format number; reworded to describe the old form without a version. Also removed the `.procoder/backlog/stories` reference in Known issues.
- vectors.md "Not yet available": no `TextCandidates`, `VectorCandidates` or `RRFK` in non-test code; section replaced with "Not available" (no built-in fusion, see fulltext.md; MySQL has no distance function). vector/doc_test.go no longer pins `k=60` and `TextCandidates`.
- docs/README.md: duplicate Interceptors and momovet entries removed; no `.procoder/specs/momo.md` reference remains; `momo generate --from-db` is already stated correctly (cmd/momo/generate_fromdb.go). Architecture entry reworded.
- ARCHITECTURE.md: rewritten from the package tree (gen/apiquery, gen/viz, momotest, momolog, importer/*, cmd/momovet, driver/sqlitedriver, sqlitepure, mysqldriver, migrate/sqlite and migrate/mysql all exist). Plan content archived to .procoder/plans/architecture-contract-archive.md.
- supported-versions.md: pgx v5.12.0 already matched go.mod (fixed in sprint 5). Fixed instead: "generated files carry the generator version in their header" (gen/header.go: format number only).
- capabilities.md: JSON row was already corrected in sprint 5. Fixed instead: Rollback row ("not built") although Engine.Rollback exists (migrate/rollback.go); `new` is wired (`new --custom`, cmd/momo/migrate.go) and Engine.Rebase exists (migrate/rebase.go); "story 098" removed; `new --custom` added to the command list.
- TYPE_MAPPING.md lines 73 to 76: already rewritten in sprint 5, kept (the SQLite note is accurate and not time-bound).
- Eight "SQLite and MySQL are not usable" sentences: already replaced in sprint 5; the story's premise (driver/ holds only pgxdriver) no longer holds, nothing to edit.
- importers.md and migrations.md: "story 228" and "story 098" references removed.

Second pass (full read of every section, sprint-6 code). Format: page:line, claim, code, fix.

- capabilities.md:381 `ErrUnsupported` row said "Anything on SQLite"; cmd/momo/migrate.go:321-326, 413-423 only refuse non-PostgreSQL for `momo migrate`; reworded to "`momo migrate` on SQLite or MySQL".
- capabilities.md Relations intro: hand-written "PostgreSQL only." contradicts the SQLite rows and the relation tests (load_integration_test.go via testopen.Each); replaced with a neutral sentence.
- capabilities.md Migrations intro: "PostgreSQL only." vs migrate/sqlite and migrate/mysql (cmd/momo/migrate.go:321-326 selects only PostgreSQL for the command); reworded to the command and a pointer to the generated status.
- capabilities.md Vectors intro: omitted SQLite (sqlite-vec, driver/sqlitedriver) that the table has; added.
- supported-versions.md:112 named `search.go` `requireIterativeSupport`; the check is vecsearch/vecsearch.go `Apply`; updated.
- supported-versions.md:38,74 CI job list omitted `bench-module` and its bench gate steps (.github/workflows/ci.yml:247-290); added.
- ai-guide.md:47 said the CLI has no `migrate new`; cmd/momo/migrate.go:861 has `new --custom`; dropped from the list.
- ai-guide.md:155 CRLF files reported stale by `--check`; cmd/momo/generate.go:34-36 counts a CRLF copy as up to date; reworded.
- ai-guide.md:323 `Config` methods omitted `IDGen` (companion.go:53); added.
- ai-guide.md:736, 848, 862 hand-written "PostgreSQL only" phrases; arrayop/arrayop.go, agg/agg.go:263, timeop/timeop.go:215-218; reworded to the behaviour (`ErrUnsupported` on SQLite and MySQL).
- ai-guide.md:1109 `Copy` ErrUnsupported on SQLite only; insert_copy.go:85-92 `noCopy` covers every driver without COPY (MySQL too); fixed.
- ai-guide.md:1110-1111 "PostgreSQL only" and `Merge` unsupported on SQLite only; insert_unnest.go:48-51, dialect/dialect.go:87 (Merge set for PostgreSQL alone); fixed.
- ai-guide.md:1690 `migrate.ErrNewerFormat`; schema/load.go:13 defines `schema.ErrNewerFormat`; fixed.
- ai-guide.md:1270 pointed to missing `example_expression_test.go`; the test is `TestExtensionOpsCustomExpressions`, expr_ops_test.go:248; fixed.
- ai-guide.md:~1849 rule 17 said every ivfflat/hnsw index needs an opclass; schema/vector.go `checkOpClass` allows ivfflat on `vector` without one; fixed.
- ai-guide.md:1424 `Joined()` "ErrUnsupported with Scope"; joinload.go:41 says Scope does not take a Loader; removed `Scope`.
- relations.md:543-546 and error table: NULL child key `ErrValidation` only for many-to-many; assoc.go:459 `childPKs` also fails has-one/has-many; fixed.
- relations.md Replace bullet and CreateGraph: nothing said about an `OnConn`-pinned handle; assoc.go:131-135 and graph.go:152-155 return `ErrUnsupported` (Op "begin"); added.
- vectors.md:7-9 example packages `momo`/`vector`; they are `momo_test`, `vector_test`, and `ExampleWith`/`ExampleApply` live in vecsearch/example_test.go; fixed.
- vectors.md:114 compile-fail path `testdata/compilefail`; it is vecop/testdata/compilefail/metricmismatch; fixed.
- vectors.md:201 "Vector indexes exist only on PostgreSQL"; dialect/mysql.go:34 and schema/vector.go:120 support MariaDB VECTOR INDEX; reworded.
- vectors.md:317 relaxed_order example used 20000 max scan tuples; vecsearch/vecsearch_integration_test.go TestFilteredANNIterativeScan uses 1,000,000; fixed.
- vectors.md:345 Explain `ErrUnsupported` on MySQL only; explain.go:22 allows PostgreSQL and SQLite only; now MySQL and MariaDB.
- vectors.md:387 `vecsearch.Apply` dialect note covered SQLite only; vecsearch/vecsearch.go `Apply`, txsetup.go:88 (SetLocal PostgreSQL only) mean every non-zero setting fails on MySQL and MariaDB; added.
- migrations.md:212 "three commands"; cmd/momo/cmd.go:46-50 has four (generate, migrate, viz, help); fixed.
- migrations.md:162 history `kind` only `sql`; migrate/postgres/session.go:30, migrate/gomigration.go also write `go`; fixed.
- migrations.md:857-860 `Engine.Rollback` "landing from a branch"; migrate/rollback.go:41, :75 exist; removed the phrase.
- migrations.md:634 `risky:` hints unconditional; migrate/diff.go:778-783 emits them on PostgreSQL only; added.

Per-section result, all other sections read in full with no discrepancies: README (intro and status, install, imports and models, MomoConfig, momo.yaml and generate, migrate, open a database, CRUD and conditional upsert, relations, similarity search, transactions, projections, command line and exit codes, More); capabilities (connection, generated blocks, core queries, writes, row locking, CTEs, window functions, hooks, secrets, RLS, PgBouncer and TLS, rest of the ErrUnsupported table); supported-versions (module, databases, Go, dependencies, OS, PostgreSQL, pgvector, PgBouncer, feature floors, stability); ai-guide (header, 1 to 12 and the observability section, apart from the fixes above); relations (1 to 9, apart from the fixes above); projections (all sections, no discrepancies); fulltext (all sections, no discrepancies); vectors (all other sections); migrations (all other sections).

Generator data: no generated block was wrong; no status.go or internal/gendocs data changed.

Nav/llms: this pass touches no mkdocs.yml and no llms generation. The earlier `Upgrading: upgrading.md` nav entry was required by TestEveryDocPageInNav for the upgrading story, which also landed on this sprint; nav structure and llms are otherwise untouched by this story.

Tests: internal/repocheck/docs_hygiene_test.go (TestDocsHaveNoPlanBookkeeping, TestArchitecturePageDescribesCode, TestDocsIndexListsEveryPage). Red run 42494821840 (probe commit b9430270a639a1e215b9f5dd5f6b33f2ada5bbd4, branch deleted): TestDocsHaveNoPlanBookkeeping flagged a [momo-05] tag and a .procoder/ reference, TestDocsIndexListsEveryPage flagged an unlinked page.

Nav: mkdocs.yml gained one entry, `Upgrading: upgrading.md` (TestEveryDocPageInNav requires every page in the nav, for the upgrading story). No structure, hook or llms change.
