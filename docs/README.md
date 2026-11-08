# Momo documentation

Index of project documentation. Each entry is added when the document is
written; see `.procoder/specs/momo.md` scope item S-21 for the full list
(architecture, API, capability matrix, migration safety and recovery,
contributor guide, supported versions, performance).

- [Guide for AI coding agents](ai-guide.md): one self-contained file for an agent using Momo in an application: setup, modeling, migrations, queries, relations, vectors, errors, testing, known pitfalls and rules. Also indexed by `llms.txt` at the repository root.
- [Go to SQL type mapping](TYPE_MAPPING.md): column type per Go type, edge values, null handling.
- [Vector search](vectors.md): pgvector types, metrics, limits, search settings and the binary codec.
- [Full-text search](fulltext.md): the tsvector column type, the match, query, rank and headline builders, and a hybrid vector plus text ranking pattern.
- [Projections](projections.md): declaring a projection, `From` and `Columns`, the generated metadata, tuple rows (`Row1` to `Row10`), the soft-delete filter.
- [Relations](relations.md): declaring, loading, filtering and writing relations, with error behavior and limits.
- [Migration safety and recovery](migrations.md): snapshots, apply semantics, locks, failure recovery, production checklist, CLI reference.
- [Interceptors](PRIVACY_RULES.md): application-level per-handle rules that add predicates or deny typed statements, the tenant recipe, and the paths they do not cover (not a security boundary).
- [Static check for raw SQL](momovet.md): the momovet analyzer that reports non-constant SQL passed to the raw-SQL functions.
- [Capabilities](capabilities.md): what each database supports, what is not built, and what returns `ErrUnsupported`.
- [Supported versions](supported-versions.md): Go, PostgreSQL, pgvector, PgBouncer, pgx and OS versions in CI, and the stability policy.

## Examples

Runnable programs, each tested end to end in CI. See [the examples index](../examples/README.md) for the layout and how to add one.

- [quickstart](../examples/quickstart/): the root README loop: models, generate, CRUD, relations, vector search, a transaction.
- [crud-zero-null](../examples/crud-zero-null/): insert, update, delete; zero value versus NULL versus default; soft delete.
- [transactions](../examples/transactions/): Tx, rollback, savepoints, read-only and isolation, retry on serialization failure.
- [relations](../examples/relations/): has-many, belongs-to, many-to-many; Load, Preload, Joined, Has, Link, CreateGraph.
- [projection-pagination](../examples/projection-pagination/): column subsets, keyset pagination with a tie-break, counting.
- [vector-search](../examples/vector-search/): HNSW index, nearest-neighbour queries, filters, search settings.
- [upgrade-database](../examples/upgrade-database/): change a model after production: add a column with a default, add an index, rename a column, apply with data kept, generate --check and drift.
- [custom-sql-migration](../examples/custom-sql-migration/): hand-written SQL migrations beside generated ones: backfill, DO $$ splitting, -- momo:no-transaction, a hand-written CONCURRENTLY index, momo.sum checksums.
- [adopt-existing-schema](../examples/adopt-existing-schema/): put a plain-SQL database under Momo with migrate baseline (no model generation from a database in v0.3.0). <!-- x-release-please-version -->
