# Vector search

Momo supports pgvector on PostgreSQL: value types, typed distance
expressions, HNSW and IVFFlat index declarations, scoped search settings,
plan inspection and an opt-in binary wire codec. This page describes what is
built. Every Go snippet below is an `Example` function in
`example_vector_test.go` (package `momo`) or `vector/example_test.go`
(package `vector`), so `go test` compiles it and checks its printed output.

## Platforms and versions

- PostgreSQL with the pgvector extension. Integration tests run against
  pgvector 0.8.7 (`pgvector/pgvector:0.8.7-pg17` primary, `0.8.7-pg16` for an
  older major). The iterative scan settings need pgvector 0.8.0 or newer.
- The `vector` extension must be installed by a role that may create it.
  Momo never installs it at connect time. A missing extension surfaces as
  `momo.ErrUnsupported` from a migration, with the extension named in the
  message. A search settings call checks the installed version the same way.

## Not yet available

These parts of the vector plan are not built on this branch, so there is no
API to document yet:

- SQLite: the plan pins sqlite-vec v0.1.6 through the Go bindings, loaded by
  `sqlitedriver.Open` and checked with `vec_version()`. There is no SQLite
  driver package yet. The dialect description already lists the SQLite
  metrics, and the distance builders already render the sqlite-vec functions
  (see the table below), but nothing opens a SQLite database with the
  extension.
- Hybrid search: PostgreSQL full-text plus vector search fused by reciprocal
  rank (planned options `TextCandidates`, `VectorCandidates` and `RRFK`, with
  k=60 as the default) does not exist yet.
- MySQL and MariaDB: no vector support.

## Value types

The `vector` package holds the four Go types. Each has a `Validate(dim)`
method, a `Value` method that produces the pgvector text form and a `Scan`
method that reads it back. Validation errors wrap `vector.ErrInvalidVector`,
which the root package re-exports as `momo.ErrInvalidVector`.

| Go type               | PostgreSQL type | Notes                                                         |
| --------------------- | --------------- | ------------------------------------------------------------- |
| `vector.Vector`       | `vector(n)`     | float32, dimension 1 to 16000 (`vector.MaxDim`)               |
| `vector.HalfVector`   | `halfvec(n)`    | float32 in Go, float16 in storage, each value within 65504    |
| `vector.SparseVector` | `sparsevec(n)`  | `Dim`, zero-based strictly increasing `Indices`, `Values`     |
| `vector.Bits`         | `bit(n)`        | `Len` bits packed most significant bit first, trailing bits 0 |

Value rules, enforced before a value reaches the server:

- The length must equal the column dimension declared on the
  `VectorColumn`.
- NaN and Inf are rejected.
- A halfvec component must satisfy |x| <= 65504.
- A dimension above 16000 is rejected for vector and halfvec.
- A sparsevec has at most 16000 stored elements (`vector.MaxSparseNNZ`) and a
  dimension up to 1,000,000,000 (`vector.MaxSparseDim`). Its text form uses
  one-based indices, as pgvector does, while `Indices` is zero-based.
- A bit string has at most `vector.MaxBits` bits, the PostgreSQL limit.

The `Example` functions `ExampleVector`, `ExampleHalfVector`,
`ExampleSparseVector` and `ExampleBits` in `vector/example_test.go` show the
validation and the text forms:

```go
v := vector.Vector{1, 2.5, -3}
fmt.Println(v.Validate(3))      // <nil>
text, _ := v.Value()
fmt.Println(text)               // [1,2.5,-3]
errors.Is(v.Validate(4), vector.ErrInvalidVector) // true
```

## Columns and distances

A `momo.VectorColumn[V]` is a typed column descriptor that carries the
declared dimension. Generated code emits one for every field of a `vector`
package type, with the dimension from the `dim` tag, so `document.Embedding`
passes straight to a distance builder. Build one by hand with
`momo.NewVectorColumn` only for a table the generator does not cover.

A vector column offers the predicates, ordering and write methods of a plain
column (`Eq`, `In`, `IsNull`, `IsNotNull`, `IsDistinctFrom`, `Asc`, `Desc`,
`Set`, `SetExpr`, `SetDefault`, `SetExcluded`), except the range comparisons
`Lt`, `Le`, `Gt`, `Ge` and `Between`: vectors have no meaningful total order,
so use a distance builder to order them. A nullable field (`*vector.Vector`)
gets `momo.NullVectorColumn`, which adds `SetNull`.

A vector field tagged `read_only`, `create_only` or `immutable` gets the
matching `ReadOnlyVectorColumn`, `CreateOnlyVectorColumn` or
`ImmutableVectorColumn`. They keep the write restriction of the plain
permission columns (no `Set` on a read-only column, an insert-only `Set` on
the other two) and every distance builder accepts them, so a read-only
embedding can be searched.

The distance builders take a column and a query vector of the same type.
Each returns a `DistanceExpr`, an `Expression[float64]` that works in the
select list, in comparisons and in `OrderBy`. `Asc()` orders nearest first
and is the default; `Desc()` orders farthest first.

| Builder                | pgvector operator | Column types            | SQLite function        |
| ---------------------- | ----------------- | ----------------------- | ---------------------- |
| `L2Distance`           | `<->`             | vector, halfvec, sparse | `vec_distance_l2`      |
| `InnerProductDistance` | `<#>`             | vector, halfvec, sparse | none                   |
| `CosineDistance`       | `<=>`             | vector, halfvec, sparse | `vec_distance_cosine`  |
| `L1Distance`           | `<+>`             | vector, halfvec, sparse | `vec_distance_l1`      |
| `HammingDistance`      | `<~>`             | bit                     | `vec_distance_hamming` |
| `JaccardDistance`      | `<%>`             | bit                     | none                   |

`InnerProductDistance` is pgvector's `<#>`, which returns the negative inner
product so that ascending order means closest. Passing a dense vector to a
bit builder, or the reverse, does not compile (`testdata/compilefail`).

The query vector is a bound argument. A dimension mismatch, NaN or an
out-of-range value is a build error wrapping `ErrValidation`, reported by
`SQL()` or by the first execution, never a server error. A metric the dialect
lacks is a build error wrapping `ErrUnsupported`. The metrics per dialect come
from `dialect.Dialect.Vector`:

```go
for _, name := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
	caps := dialect.For(name).Vector
	fmt.Println(name, caps.Metrics, caps.Indexes)
}
// postgres [l2 ip cosine l1 hamming jaccard] [hnsw ivfflat]
// sqlite [l2 l1 cosine hamming] []
```

A nearest-neighbour query (`ExampleCosineDistance`):

```go
c, err := momo.Select[example.Document](db).
	OrderBy(momo.CosineDistance(document.Embedding, query).Asc()).
	Limit(5).
	SQL()
// ... ORDER BY ("documents"."embedding" <=> $1) ASC LIMIT $2
```

Reading the distance next to a column subset needs a projection struct
(`momo.Projection`, `From` and `Columns`). A projection does not get the
automatic soft-delete filter: on a table with a soft-delete column add
`.Where(document.DeletedAt.IsNull())` yourself, or soft-deleted rows appear in
the results.

## Declaring vector columns and indexes

The generator maps `vector.Vector`, `vector.HalfVector`,
`vector.SparseVector` and `vector.Bits` fields to `vector`, `halfvec`,
`sparsevec` and `bit` columns. The `dim` tag sets the dimension, and
sparsevec and bit require it:

```go
Embedding vector.Vector `momo:"dim:1536"`
```

Generation adds `CREATE EXTENSION vector` to the schema when a vector column
or index is present. An ANN index needs an operator class, and pgvector's
hnsw has no default one, so declare vector indexes in `MomoConfig` (the
reference model is `internal/testmodels/example/models.go`):

```go
func (d *Document) MomoConfig(c *momo.Config) {
	c.Index("documents_embedding_hnsw", &d.Embedding).
		Method("hnsw").
		Opclass(&d.Embedding, "vector_cosine_ops").
		With("m", "16").
		With("ef_construction", "64")
}
```

The generator and `schema.ValidateVectorIndex` reject an index that
pgvector would refuse, before any SQL runs. The error wraps
`schema.ErrInvalidVectorIndex`. `schema.VectorIndexOpClass(type, metric)`
returns the opclass for a column type and metric (`ExampleValidateVectorIndex`).

### Index limits

| Method  | Types                           | Dimension limit                      | Opclasses (suffix after `<type>_`)        |
| ------- | ------------------------------- | ------------------------------------ | ----------------------------------------- |
| hnsw    | vector, halfvec, sparsevec, bit | vector 2000, halfvec 4000, bit 64000 | l2, ip, cosine, l1; bit: hamming, jaccard |
| ivfflat | vector, halfvec, bit            | vector 2000, halfvec 4000, bit 64000 | l2, ip, cosine; bit: hamming              |

- ivfflat does not support sparsevec.
- hnsw accepts the storage parameters `m` (2 to 100, default 16) and
  `ef_construction` (4 to 1000, default 64, at least 2*m).
- ivfflat requires an explicit `lists` parameter (1 to 32768). Momo does not
  rely on the server default of 100.
- An index covers one plain column. Expression parts are rejected.
- Vector indexes exist only on PostgreSQL. Another dialect gets an error.

## Migrations

`migrate.Diff` emits the extension, the table and the index, and
`migrate/postgres` renders them (`Example_vectorMigration`):

```text
CREATE EXTENSION IF NOT EXISTS "vector"
CREATE TABLE "documents" ("id" bigint NOT NULL, "embedding" vector(3) NOT NULL, ...)
CREATE INDEX "documents_embedding_hnsw" ON "documents" USING "hnsw" ("embedding" "vector_cosine_ops") WITH ("m" = 16, "ef_construction" = 64)
```

Inspection reads method, operator class and storage parameters back, so an
unchanged vector index produces no diff. A default operator class (ivfflat on
vector, `vector_l2_ops`) is stored as empty, which matches inspection.

## Search settings

`momo.SearchSettings` applies pgvector settings for one transaction. Zero
values leave the server default alone. Settings use
`set_config(name, value, true)`, so they revert at commit or rollback and
never reach a pooled connection, which makes them safe behind PgBouncer in
transaction mode.

| Field                  | Setting                                            | Range                                                           |
| ---------------------- | -------------------------------------------------- | --------------------------------------------------------------- |
| `HNSWEfSearch`         | `hnsw.ef_search`                                   | 1 to 1000                                                       |
| `HNSWIterativeScan`    | `hnsw.iterative_scan`                              | `IterativeOff`, `IterativeRelaxedOrder`, `IterativeStrictOrder` |
| `HNSWMaxScanTuples`    | `hnsw.max_scan_tuples`                             | 1 to 2147483647 (server default 20000)                          |
| `IVFFlatProbes`        | `ivfflat.probes`                                   | 1 to 32768                                                      |
| `IVFFlatIterativeScan` | `ivfflat.iterative_scan`                           | `IterativeOff` or `IterativeRelaxedOrder`                       |
| `IVFFlatMaxProbes`     | `ivfflat.max_probes`                               | 1 to 32768                                                      |
| `ExactSearch`          | `enable_indexscan`, `enable_bitmapscan` set to off | bool                                                            |

`SearchSettings.Validate` reports a value outside its range as
`ErrValidation`. The iterative fields, `HNSWMaxScanTuples` and
`IVFFlatMaxProbes` need pgvector 0.8.0 or newer. When any of them is set,
`SetSearch` reads `pg_extension` and returns `ErrUnsupported` on an older or
missing extension, instead of letting the server accept an unknown setting as
a placeholder.

Use `DB.WithSearch` for a block, or `Tx.SetSearch` inside a transaction you
already have (`ExampleDB_WithSearch`, `ExampleTx_SetSearch`):

```go
settings := momo.SearchSettings{
	HNSWEfSearch:      100,
	HNSWIterativeScan: momo.IterativeRelaxedOrder, // needs pgvector 0.8
	HNSWMaxScanTuples: 20000,
}
err := db.WithSearch(ctx, settings, func(tx *momo.Tx) error {
	_, err := momo.Select[example.Document](tx).
		OrderBy(momo.CosineDistance(document.Embedding, query).Asc()).
		Limit(10).
		All(ctx)
	return err
})
```

`Tx.SetLocal(ctx, name, value)` sets any run-time parameter the same way.
Never pass an untrusted name: the name is bound, but `set_config` can change
`role` or `search_path`.

### Exact versus approximate search

An index scan is approximate. `ExactSearch` turns off index and bitmap scans
for the rest of the transaction, so a vector `ORDER BY` scans the table and
returns the exact nearest neighbours. The switches are not specific to
vector indexes: every other index lookup in that transaction also falls back
to a sequential scan, so keep the transaction short and dedicated to the
exact query.

### Filters, iterative scans and result counts

With a selective filter and `LIMIT k`, pgvector with iterative scan off scans
the index once and applies the filter afterwards, so it can return
fewer than k rows. `TestFilteredANNIterativeScan` reproduces this: a filter that matches 1%
of 20000 rows returns fewer than 10 rows with iterative scan off, and 10 with
`relaxed_order` and `HNSWMaxScanTuples` 20000.

`relaxed_order` keeps scanning until enough rows pass the filter or
`hnsw.max_scan_tuples` is reached. The result is therefore still approximate,
and it can be shorter than k when the filter matches fewer rows than that
bound allows. `strict_order` (hnsw only) keeps exact distance order while it
scans.

## Verifying index use

`SelectQuery.Explain(ctx)` runs `EXPLAIN` (never `EXPLAIN ANALYZE`), binds
the parameters and returns the plan text. It does not execute the query. Run
it inside the transaction whose settings you want to see
(`ExampleSelectQuery_Explain`):

```go
plan, err := momo.Select[example.Document](db).
	OrderBy(momo.L2Distance(document.Embedding, query).Asc()).
	Limit(10).
	Explain(ctx)
// look for "Index Scan using documents_embedding_hnsw"
```

`TestVectorIndexUsed` and `TestExactSearchBypassesVectorIndex` use this
pattern to assert that the index is used, and that `ExactSearch` stops using
it. Expect a sequential scan on a small table, because the planner prefers it
below a few thousand rows. Explain returns `ErrUnsupported` on MySQL.

## PostgreSQL wire format

By default vector values travel as text. The binary codec is opt-in and lives
in `driver/pgxdriver`:

- `pgxdriver.ConfigureVectors(cfg)` installs an `AfterConnect` hook that
  looks up the OIDs of `vector`, `halfvec` and `sparsevec` and registers
  codecs for them and for `bit`. Call it before `pgxpool.NewWithConfig`. The
  lookup runs once per connection: a connection opened before
  `CREATE EXTENSION vector` stays on text until the pool recycles it.
- `pgxdriver.WithVectorFormat(pgxdriver.VectorBinary)` asks for binary
  parameters. It needs a describe-based exec mode (`WithExecMode` with
  `pgx.QueryExecModeDescribeExec`, `CacheStatement` or `CacheDescribe`);
  pgx's default `QueryExecModeExec` always sends text, with identical
  results.
- `COPY` always uses binary and so needs `ConfigureVectors`.
- Decoders bound every length they read before they allocate.

`ExampleConfigureVectors` shows the setup:

```go
cfg, err := pgxpool.ParseConfig("postgres://localhost/app")
pgxdriver.ConfigureVectors(cfg)
pool, err := pgxpool.NewWithConfig(ctx, cfg)
db := momo.New(pgxdriver.New(pool, pgxdriver.WithVectorFormat(pgxdriver.VectorBinary)))
```

## Differences between approaches

- pgvector exact search (`ExactSearch`) reads every row and returns the true
  nearest neighbours. Index search (hnsw, ivfflat) is approximate and tuned
  with `hnsw.ef_search`, `ivfflat.probes` and the iterative settings.
- sqlite-vec is brute-force KNN with no ANN index. A request for an ANN index
  on SQLite is an error. SQLite supports float32 vectors (l2, l1, cosine) and
  bit vectors (hamming) only; halfvec and sparsevec arguments, inner product
  and Jaccard return `ErrUnsupported` at build time. SQLite bit
  vectors keep only the packed bytes, so the declared column length is
  authoritative for `Len`. See "Not yet available" for the driver status.
- `SetSearch` on SQLite: `ExactSearch` alone is a no-op, every search is
  already exact, and any other field is `ErrUnsupported`.
- MariaDB VECTOR INDEX is approximate; MySQL and MariaDB are not supported yet.
