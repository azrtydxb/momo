# Sprint 2: Generator from Go structs, with its type prerequisites

Status: closed 2026-11-06
Created: 2026-11-05

## Goal

A developer can write ordinary Go structs, with `momo:` tags and a `MomoConfig` companion, run `momo generate`, and get reflection-free, compiling model code:

- column and relation descriptors;
- `Meta` with typed hooks wiring left for momo-05;
- lookup by name;
- deterministic, header-marked output that refuses to overwrite handwritten files;
- naming presets matching GORM, Bun and Ent;
- the generator plugin interface;
- the `momo.yaml` config.

This sprint also lands the three type-only prerequisites the generator consumes: momo-09 Task 1 (QueryEvent pool-wait fields and `RecordPoolWait`), momo-04 Task 1 (schema managed-object types) and momo-06 Task 2 (the `Relation` descriptor).

Out of scope, because they wait for momo-05's write builders and `HookSet`: momo-02 Tasks 25 (gate), 26, 27, 29 and 30.

Done means every pulled story is closed and the generator golden tests, the example-model build and the full race suite pass, plus the PostgreSQL integration gate.

## Result

committed: 115
done: 105 (20261104-024-momo-09-task-1-queryevent-pool-wait-transaction, 20261104-025-momo-02-task-1-companion-dsl-in-the-root-runtime, 20261104-026-momo-02-task-2-struct-tag-grammar-with-the-full, 20261104-027-momo-02-task-3-momo-naming-preset, 20261104-028-momo-02-task-4-record-gorm-bun-and-ent-naming-fi, 20261104-029-momo-02-task-5-gorm-bun-and-ent-naming-presets-m, 20261104-030-momo-02-task-6-package-loading-with-positioned-d, 20261104-031-momo-02-task-7-model-discovery-from-the-momo-mod, 20261104-032-momo-02-task-8-embedded-structs-prefixes-and-mix, 20261104-033-momo-02-task-9-statically-read-momoconfig-compan, 20261104-034-momo-02-task-10-schema-construction-and-tag-effe, 20261104-035-momo-02-task-11-go-to-sql-type-map-for-the-gener, 20261104-036-momo-02-task-12-field-write-permissions-s-71-wit, 20261104-037-momo-02-task-13-column-and-relation-descriptors, 20261104-038-momo-02-task-14-momometa-emission-in-the-model-p, 20261104-039-momo-02-task-15-generated-columnbyname-allowlist, 20261104-040-momo-02-task-16-identifier-collisions-and-reserv, 20261104-041-momo-02-task-17-the-writer-collision-safe-determ, 20261104-042-momo-02-task-18-emit-pipeline-and-determinism, 20261104-043-momo-02-task-19-cross-package-generation-without, 20261104-044-momo-02-task-20-generator-plugin-interface-s-62, 20261104-045-momo-02-task-21-momo-yaml-configuration-and-nami, 20261104-046-momo-02-task-22-example-models-with-go-generate, 20261104-047-momo-02-task-23-cli-momo-generate-and-check-and, 20261104-052-momo-02-task-28-relation-descriptor-emission-pla, 20261104-055-momo-02-task-31-generated-header-convention-and, 20261104-056-momo-03-task-1-testopen-package-with-postgres-an, 20261104-057-momo-03-task-2-optional-driver-capability-interf, 20261104-058-momo-03-task-3-error-classification-for-postgres, 20261104-059-momo-03-task-4-pgx-adapter-core-pgxpool-with-per, 20261104-060-momo-03-task-5-pool-acquire-tracer-feeding-momo, 20261104-061-momo-03-task-6-transactions-with-isolation-read, 20261104-062-momo-03-task-7-pinned-connections-and-pool-stati, 20261104-063-momo-03-task-8-copy-and-batch-paths-on-postgresq, 20261104-064-momo-03-task-9-database-sql-adoption-fromsqldb-a, 20261104-065-momo-03-task-10-pgbouncer-transaction-mode-compa, 20261104-067-momo-03-task-12-iterator-closes-the-cursor-on-ea, 20261104-069-momo-03-task-14-serverversion-for-merge-gating, 20261104-070-momo-04-task-1-schema-types-for-snapshots-and-ma, 20261104-071-momo-04-task-2-schema-normalization-and-fuzznorm, 20261104-072-momo-04-task-3-migrate-core-types-errors-options, 20261104-073-momo-04-task-4-migration-file-discovery-directiv, 20261104-074-momo-04-task-5-momo-sum-checksum-file, 20261104-075-momo-04-task-6-snapshot-wrapper-with-parent-vers, 20261104-076-momo-04-task-7-offline-chain-numbering-and-valid, 20261104-078-momo-04-task-9-dependency-aware-ordering-of-mana, 20261104-079-momo-04-task-10-postgresql-inspection-schemas-ex, 20261104-080-momo-04-task-11-postgresql-inspection-indexes-ex, 20261104-081-momo-04-task-12-postgresql-inspection-views-func, 20261104-082-momo-04-task-13-postgresql-render-for-all-change, 20261104-083-momo-04-task-14-postgresql-migration-lock-and-se, 20261104-084-momo-04-task-15-runner-history-table-failure-rec, 20261104-085-momo-04-task-16-engine-apply-plan-status-with-lo, 20261104-086-momo-04-task-17-diff-writes-files-no-change-roun, 20261104-087-momo-04-task-18-baseline-and-drift, 20261104-089-momo-04-task-20-automigrate-additive, 20261104-102-momo-05-task-1-integration-fixtures-capabilities, 20261104-103-momo-05-task-2-transactions-error-classification, 20261104-104-momo-05-task-3-safe-names-and-like-escaping-s-43, 20261104-105-momo-05-task-4-joins-aliases-and-self-joins, 20261104-106-momo-05-task-5-projections-aggregates-group-by-a, 20261104-107-momo-05-task-6-subqueries-ctes-including-recursi, 20261104-108-momo-05-task-7-keyset-pagination-integration-tes, 20261104-109-momo-05-task-8-locking-optimizer-hints-and-runti, 20261104-110-momo-05-task-9-returning-on-insert-update-and-de, 20261104-111-momo-05-task-10-raw-sql-arguments-named-expanded, 20261104-113-momo-05-task-12-insert-builder-single-model-bulk, 20261104-114-momo-05-task-13-bulk-insert-chunking-under-the-b, 20261104-116-momo-05-task-15-update-builder-mutation-guardrai, 20261104-117-momo-05-task-16-explicit-write-states-omitted-ze, 20261104-118-momo-05-task-17-delete-builder-and-guardrail-s-5, 20261104-119-momo-05-task-18-soft-delete-variants-s-75-and-th, 20261104-120-momo-05-task-19-upsert-by-conflict-target-s-5, 20261104-121-momo-05-task-20-automatic-timestamps-and-the-inj, 20261104-122-momo-05-task-21-go-side-defaults-and-validators, 20261104-136-momo-05-task-35-shared-suite-groups-runall-and-t, 20261104-138-momo-06-task-1-normalized-key-encoding, 20261104-139-momo-06-task-2-relation-descriptor-type-and-vali, 20261104-140-momo-06-task-3-relation-fixture-models-schema-an, 20261104-141-momo-06-task-4-tuple-in-predicate-and-bind-limit, 20261104-142-momo-06-task-5-eager-loading-core-for-belongs-to, 20261104-143-momo-06-task-6-has-many-loading-stable-order-par, 20261104-144-momo-06-task-7-composite-keys, 20261104-145-momo-06-task-8-bind-limit-chunking, 20261104-146-momo-06-task-9-many-to-many-through-a-join-table, 20261104-147-momo-06-task-10-polymorphic-self-referencing-bac, 20261104-149-momo-06-task-12-top-n-per-parent-with-a-window-f, 20261104-150-momo-06-task-13-preload-on-selectquery, 20261104-151-momo-06-task-14-pipelined-relation-loading-over, 20261104-152-momo-06-task-15-single-query-join-loading-for-to, 20261104-153-momo-06-task-16-relation-predicates-traversal-an, 20261104-154-momo-06-task-17-association-helpers-link-unlink, 20261104-155-momo-06-task-18-graph-writes-creategraph-findorc, 20261104-156-momo-07-task-1-vector-value-types-and-validation, 20261104-157-momo-07-task-2-dialect-vector-capabilities-and-d, 20261104-158-momo-07-task-3-vectorcolumn-and-typed-distance-e, 20261104-159-momo-07-task-4-vector-index-declarations-and-val, 20261104-160-momo-07-task-5-postgresql-vector-ddl-round-trip, 20261104-161-momo-07-task-6-scoped-search-settings-exact-sear, 20261104-162-momo-07-task-7-explain-inspection-and-index-usag, 20261104-197-momo-07-task-12-vector-documentation, 20261105-generate-check-reports-crlf-checkouts-as-stale-on-windows, 20261105-generator-emits-vectorcolumn-for-vector-fields, 20261105-getting-started-readme-for-v0-1-0, 20261106-conditional-upsert-where-on-on-conflict-target-and-do-update)
carried: 10 (20261104-048-momo-02-task-24-generated-code-size-and-compile, 20261104-077-momo-04-task-8-pure-diff-engine-with-explicit-re, 20261104-094-momo-04-task-25-postgresql-end-to-end-apply-with, 20261104-097-momo-04-task-28-cli-url-redaction-and-tls-warnin, 20261104-098-momo-04-task-29-cli-wiring-for-every-migrate-com, 20261104-123-momo-05-task-22-codecs-and-the-types-package-s-7, 20261104-124-momo-05-task-23-documented-type-mapping-with-rou, 20261104-148-momo-06-task-11-nested-filtered-ordered-eager-lo, 20261104-163-momo-07-task-8-pgx-binary-codecs-for-all-four-ve, 20261104-165-momo-07-task-10-pinned-pgvector-versions-and-ver)

## Retro

What slowed us down:

- Running the full suite on the Mac filled the USB disk with Go caches and loaded the machine; the USB disk also dropped out once mid-run, leaving two agents unable to commit.
- Repository tests encode standards (version-tagged actions, README snippets compiled in the quickstart, permission columns mirroring Column) that a change can break without the author knowing; each cost a CI round.
- Story criteria were not checked against the code until sprint close, so six stories looked open that were partly or fully done.
- Scout findings were counted from a status-filtered list that undercounted.

What we change next sprint:

- Agents run only targeted tests locally and push; CI runs the full matrix (now standing practice).
- Agent briefs name the repository tests that guard the area they touch (repocheck, readme snippets, permission mirrors).
- A story closes in the same branch as its code, with evidence, not at sprint end.
- Scout checks start from repository metrics and reconcile counts.

One adaptation worth keeping:

- Build a scratch application from the docs alone (the AI-guide validation run) before calling documentation done; it found a generator-breaking example and an undocumented soft-delete gap that reading never caught.
