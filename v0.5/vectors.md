# Vector search

Momo supports pgvector on PostgreSQL: value types, typed distance
expressions, HNSW and IVFFlat index declarations, scoped search settings,
plan inspection and an opt-in binary wire codec. It also runs the float32 and
bit vector builders on SQLite with sqlite-vec, and the distance builders on
MariaDB. This page describes what is built. Every Go snippet below is an `Example` function in
`col_example_vector_test.go` (package `momo_test`), `vector/example_test.go`
(package `vector_test`) or `vecsearch/example_test.go` (package
`vecsearch_test`), so `go test` compiles it and checks its printed output.

## Platforms and versions

- SQLite with sqlite-vec v0.1.6, through `driver/sqlitedriver` (cgo, not
  Windows); see [SQLite and sqlite-vec](#sqlite-and-sqlite-vec) below.
- PostgreSQL with the pgvector extension. Integration tests run against
  pgvector 0.8.7 (`pgvector/pgvector:0.8.7-pg17` primary, `0.8.7-pg16` for an
  older major). The iterative scan settings need pgvector 0.8.0 or newer.
- The `vector` extension must be installed by a role that may create it.
  Momo never installs it at connect time. A missing extension surfaces as
  `momo.ErrUnsupported` from a migration, with the extension named in the
  message. A search settings call checks the installed version the same way.

## Not available

- Hybrid search on SQLite and MySQL: `momo.HybridSearch` needs PostgreSQL and
  returns `ErrUnsupported` elsewhere; see [Hybrid search](#hybrid-search).
- MySQL: the server stores vectors but has no distance function, so a distance
  query fails with `ErrUnsupported`; see "MariaDB and MySQL" below. The
  `momo migrate` command does not run `migrate/mysql`, which renders its
  statements as-is, so a MySQL or MariaDB `VECTOR` column is created by your
  own DDL.

## Value types

The `vector` package holds the four Go types. Each has a `Validate(dim)`
method, a `Value` method that produces the pgvector text form and a `Scan`
method that reads it back. Validation errors wrap `vector.ErrInvalidVector`,
which the root package re-exports as `momo.ErrInvalidVector`.

| Go type               | PostgreSQL type | Notes                                                                         |
| --------------------- | --------------- | ----------------------------------------------------------------------------- |
| `vector.Vector`       | `vector(n)`     | float32, dimension 1 to 16000 (`vector.MaxDim`)                               |
| `vector.HalfVector`   | `halfvec(n)`    | float32 in Go, float16 in storage, each value within 65504                    |
| `vector.SparseVector` | `sparsevec(n)`  | `Dim`, zero-based strictly increasing `Indices`, `vector.SparseVector.Values` |
| `vector.Bits`         | `bit(n)`        | `Len` bits packed most significant bit first, trailing bits 0                 |

Value rules, enforced before a value reaches the server:

- The length must equal the column dimension declared on the
  `VectorColumn`, unless the column is dimensionless (see
  [Dimensionless columns](#dimensionless-columns)).
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

<!-- docscheck:fragment -->

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
The builders live in package `github.com/azrtydxb/momo/vecop`. Each returns a
`vecop.Expr`, an `Expression[float64]` that works in the
select list, in comparisons and in `OrderBy`. `Asc()` orders nearest first
and is the default; `Desc()` orders farthest first.

| Builder              | pgvector operator | Column types            | SQLite function        |
| -------------------- | ----------------- | ----------------------- | ---------------------- |
| `vecop.L2`           | `<->`             | vector, halfvec, sparse | `vec_distance_l2`      |
| `vecop.InnerProduct` | `<#>`             | vector, halfvec, sparse | none                   |
| `vecop.Cosine`       | `<=>`             | vector, halfvec, sparse | `vec_distance_cosine`  |
| `vecop.L1`           | `<+>`             | vector, halfvec, sparse | `vec_distance_l1`      |
| `vecop.Hamming`      | `<~>`             | bit                     | `vec_distance_hamming` |
| `vecop.Jaccard`      | `<%>`             | bit                     | none                   |

`vecop.InnerProduct` is pgvector's `<#>`, which returns the negative inner
product so that ascending order means closest. Passing a dense vector to a
bit builder, or the reverse, does not compile (`vecop/testdata/compilefail`).

The query vector is a bound argument. A dimension mismatch (except on a
dimensionless column, see [Dimensionless columns](#dimensionless-columns)),
NaN or an out-of-range value is a build error wrapping `ErrValidation`, reported by
`SQL()` or by the first execution, never a server error. A metric the dialect
lacks is a build error wrapping `ErrUnsupported`. The metrics per dialect come
from `dialect.Dialect.Vector`:

<!-- docscheck:fragment -->

```go
for _, name := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
	caps := dialect.For(name).Vector
	fmt.Println(name, caps.Metrics, caps.Indexes)
}
// postgres [l2 ip cosine l1 hamming jaccard] [hnsw ivfflat]
// sqlite [l2 l1 cosine hamming] []
```

A nearest-neighbour query (`Example_vectorNearest`):

<!-- docscheck:fragment -->

```go
c, err := momo.Select[example.Document](db).
	OrderBy(vecop.Cosine(document.Embedding, query).Asc()).
	Limit(5).
	SQL()
// ... ORDER BY ("documents"."embedding" <=> $1) ASC LIMIT $2
```

Reading the distance next to a column subset needs a projection struct
(`momo.Projection`, `From` and `Columns`; see [projections](projections.md)).
A projection read `From` a table with a soft-delete column gets the same
automatic filter as a model, so soft-deleted rows stay out of the results.
Add the predicate yourself only when `From` is a subquery or a hand-built
`TableRef`.

## Declaring vector columns and indexes

The generator maps `vector.Vector`, `vector.HalfVector`,
`vector.SparseVector` and `vector.Bits` fields to `vector`, `halfvec`,
`sparsevec` and `bit` columns. The `dim` tag sets the dimension, and
sparsevec and bit require it:

<!-- docscheck:fragment -->

```go
Embedding vector.Vector `momo:"dim:1536"`
```

Generation adds `CREATE EXTENSION vector` to the schema when a vector column
or index is present. An ANN index needs an operator class, and pgvector's
hnsw has no default one, so declare vector indexes in `MomoConfig` (the
reference model is `internal/testmodels/example/models.go`):

<!-- docscheck:fragment -->

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
- The hnsw and ivfflat methods are pgvector's. `schema.ValidateVectorIndex`
  rejects them for another dialect, with an error that wraps
  `schema.ErrInvalidVectorIndex`. MariaDB has its own `VECTOR INDEX`; see
  "MariaDB and MySQL" below.

### Dimensionless columns

A `vector.Vector` or `vector.HalfVector` field without a `dim` tag maps to the
bare `vector` or `halfvec` type. Use it for a store whose embedding size
depends on the configured model: each row carries its size in a `dims`
column, and every search filters on it.

<!-- docscheck:fragment -->

```go
type KbChunk struct {
	momo.Model `momo:"table:kb_chunks"`
	ID         int64         `momo:"pk,identity"`
	Dims       int
	Embedding  vector.Vector // no dim: any size
}
```

- Generation emits a plain `vector` column and a `momo.VectorColumn` whose
  `Dim()` is zero. `sparsevec` and `bit` still require `dim`.
- A distance argument of any size is accepted; it is checked against its own
  size, so NaN, Inf and an empty vector are still rejected. The column never
  catches a size mismatch itself.
- pgvector fails a distance between two sizes, so filter on `dims` in the same
  query: `Where(chunk.Dims.Eq(len(q)))` plus `OrderBy(vecop.Cosine(chunk.Embedding, q).Asc())`.
- The migration round-trips the bare type and the next diff is empty.
- Limit: pgvector cannot build an hnsw or ivfflat index on a dimensionless
  column. Momo refuses one, with an error that wraps
  `schema.ErrInvalidVectorIndex` and names the column. Searches run as a
  sequential scan over the filtered rows. An expression or partial index such
  as `((embedding::vector(1536))) WHERE dims = 1536` is not supported yet:
  expression parts are rejected.

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

`vecsearch.Settings` (package `github.com/azrtydxb/momo/vecsearch`) applies pgvector settings for one transaction. Zero
values leave the server default alone. Settings use
`set_config(name, value, true)`, so they revert at commit or rollback and
never reach a pooled connection, which makes them safe behind PgBouncer in
transaction mode.

| Field                   | Setting                                            | Range                                                                                         |
| ----------------------- | -------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `HNSWEfSearch`          | `hnsw.ef_search`                                   | 1 to 1000                                                                                     |
| `HNSWIterativeScan`     | `hnsw.iterative_scan`                              | `vecsearch.IterativeOff`, `vecsearch.IterativeRelaxedOrder`, `vecsearch.IterativeStrictOrder` |
| `HNSWMaxScanTuples`     | `hnsw.max_scan_tuples`                             | 1 to 2147483647 (server default 20000)                                                        |
| `HNSWScanMemMultiplier` | `hnsw.scan_mem_multiplier`                         | 1 to 1000, float, a multiple of `work_mem`                                                    |
| `IVFFlatProbes`         | `ivfflat.probes`                                   | 1 to 32768                                                                                    |
| `IVFFlatIterativeScan`  | `ivfflat.iterative_scan`                           | `vecsearch.IterativeOff` or `vecsearch.IterativeRelaxedOrder`                                 |
| `IVFFlatMaxProbes`      | `ivfflat.max_probes`                               | 1 to 32768                                                                                    |
| `ExactSearch`           | `enable_indexscan`, `enable_bitmapscan` set to off | bool                                                                                          |

`vecsearch.Settings.Validate` reports a value outside its range as
`ErrValidation`. The iterative fields, `HNSWMaxScanTuples`,
`HNSWScanMemMultiplier` and `IVFFlatMaxProbes` need pgvector 0.8.0 or newer. When any of them is set,
`vecsearch.Apply` reads `pg_extension` and returns `ErrUnsupported` on an older or
missing extension, instead of letting the server accept an unknown setting as
a placeholder.

Use `vecsearch.With` for a block, or `vecsearch.Apply` inside a transaction you
already have (`ExampleWith`, `ExampleApply`):

<!-- docscheck:fragment -->

```go
settings := vecsearch.Settings{
	HNSWEfSearch:      100,
	HNSWIterativeScan: vecsearch.IterativeRelaxedOrder, // needs pgvector 0.8
	HNSWMaxScanTuples: 20000,
	HNSWScanMemMultiplier: 2, // pgvector 0.8: iterative scan memory, times work_mem
}
err := vecsearch.With(ctx, db, settings, func(tx *momo.Tx) error {
	_, err := momo.Select[example.Document](tx).
		OrderBy(vecop.Cosine(document.Embedding, query).Asc()).
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
`relaxed_order` and `HNSWMaxScanTuples` 1,000,000.

`relaxed_order` keeps scanning until enough rows pass the filter or
`hnsw.max_scan_tuples` is reached. The result is therefore still approximate,
and it can be shorter than k when the filter matches fewer rows than that
bound allows. `strict_order` (hnsw only) keeps exact distance order while it
scans.

## Hybrid search

`momo.HybridSearch` fuses a PostgreSQL full-text leg and a cosine-distance
vector leg with reciprocal rank fusion (RRF) in one statement. Each leg picks
its own best `TextCandidates` and `VectorCandidates` rows (default 100, at most
1000), ranks them, and a row scores the sum of `1/(RRFK+rank)` over the legs it
appears in (`RRFK` defaults to 60 and must be at least 1). The text leg ranks
with `ts_rank_cd` over `websearch_to_tsquery`; the vector leg uses `<=>`.
Ties break on the primary key ascending, in each leg's ranking and in the final
order, so the result does not depend on insertion order. The vector leg orders
by distance alone so an hnsw or ivfflat index can serve it; when rows tie on
distance exactly at the candidate cut-off, which of them become candidates is
not defined.

<!-- docscheck:fragment -->

```go
hits, err := momo.HybridSearch[Article](db, momo.HybridSpec{
	Text:        articleBody,
	TextConfig:  "english",
	Query:       userInput,
	Vector:      articleEmb,
	QueryVector: queryEmbedding,
	Limit:       10,
}).All(ctx)
// each hit has .Row (the model) and .Score
```

The model needs a single-column primary key, `Query` must not be empty (use a
plain vector query for that), and `HybridSpec.Text` and `HybridSpec.Vector` must be columns of the
model's table; otherwise the call returns `ErrValidation`. Soft-deleted rows are
left out. The vector leg takes the dense `vector` type only (not `halfvec`,
`sparsevec` or `bit`) and skips rows with a NULL embedding and rows whose
cosine distance is NaN (zero vectors).
`HybridQuery.SQL` returns the statement and its arguments without running it.
The vector leg can use an hnsw or ivfflat index on the vector column with
`vector_cosine_ops`. The text leg binds the configuration as a parameter, and a
GIN expression index on `to_tsvector('english', body)` needs a constant
configuration in its definition, so PostgreSQL may not match it to a bound
`regconfig`; check the plan with EXPLAIN. Without an
index a leg scans the table, so the candidate counts bound the work after the
scan, not the scan itself. For
a different mix of signals, write your own score as shown in
[full-text search](fulltext.md#hybrid-ranking-vector-plus-text).

## Verifying index use

`SelectQuery.Explain(ctx)` runs `EXPLAIN` (never `EXPLAIN ANALYZE`), binds
the parameters and returns the plan text. It does not execute the query. Run
it inside the transaction whose settings you want to see
(`ExampleSelectQuery_Explain`):

<!-- docscheck:fragment -->

```go
plan, err := momo.Select[example.Document](db).
	OrderBy(vecop.L2(document.Embedding, query).Asc()).
	Limit(10).
	Explain(ctx)
// look for "Index Scan using documents_embedding_hnsw"
```

`TestVectorIndexUsed` and `TestExactSearchBypassesVectorIndex` use this
pattern to assert that the index is used, and that `ExactSearch` stops using
it. Expect a sequential scan on a small table, because the planner prefers it
below a few thousand rows. Explain returns `ErrUnsupported` on MySQL and MariaDB.

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

<!-- docscheck:fragment -->

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
  on SQLite is `ErrUnsupported`. SQLite supports float32 vectors (l2, l1, cosine) and
  bit vectors (hamming) only; halfvec and sparsevec arguments, inner product
  and Jaccard return `ErrUnsupported` at build time. SQLite bit
  vectors keep only the packed bytes, so the declared column length is
  authoritative for `Len`. See [SQLite and sqlite-vec](#sqlite-and-sqlite-vec).
- `vecsearch.Apply` on SQLite: `ExactSearch` alone is a no-op, every search is
  already exact, and any other field is `ErrUnsupported`. On MySQL and MariaDB
  every non-zero setting, `ExactSearch` included, is `ErrUnsupported`, because
  `Tx.SetLocal` has no transaction-scoped setting there.
- MariaDB VECTOR INDEX is approximate; see "MariaDB and MySQL" below.

## SQLite and sqlite-vec

`driver/sqlitedriver` links sqlite-vec v0.1.6 (the version `vec_version()`
reports; `sqlitedriver.SQLiteVecVersion` is the release it is tested with, and
`Open` fails with `ErrUnsupported` on another minor or major version but accepts
a later patch release of the same minor). The same typed builders that serve pgvector
render sqlite-vec's scalar functions, and the integration tests run them
against a real SQLite database: `col_vector_sqlite_integration_test.go` in the root
package and `TestSQLiteVectorParity` in `driver/sqlitedriver`. It needs cgo and
does not build on Windows. The SQLite migration dialect (`migrate/sqlite`) is library only, and the
generator refuses vector fields on SQLite, so you create the table with your
own DDL.

**Columns.** A vector is a plain `BLOB` column; sqlite-vec has no declared
dimension, so the length is guarded by a `CHECK`. `momo.SQLiteVectorType` and
`momo.SQLiteBitsType` return the definition to put after the column name:
`BLOB CHECK (length("emb") = 12)` for three float32 elements and
`BLOB CHECK (length("bits") = 1)` for eight bits. A vector of the wrong size
is then refused by the database as `ErrCheckViolation`. A dimensionless column
is `ErrUnsupported`, because there is no length to check.

**Values.** `vector.Vector` is stored as a little-endian float32 blob and read
back from one, and `vector.Bits` as its packed bytes (most significant bit first, as in
pgvector: bit 0 is the top bit of byte 0). Both are validated before
they are bound (NaN, Inf, an empty vector, a bad bit length are
`ErrInvalidVector`), because a blob has no text form that would reject them
later. A `*vector.Vector` or `*vector.Bits` field is nullable: nil binds `NULL`.
`vector.HalfVector` and `vector.SparseVector` have no sqlite-vec type, so binding one, or
searching with one, is `ErrUnsupported` when the query is built. A bit vector
read from SQLite has a `Len` of 8 times its byte count, so the column length is
the authority for a length that is not a multiple of 8.

**Metrics.** These are exactly the functions v0.1.6 registers:

| Builder              | SQLite function                                            | Column type     |
| -------------------- | ---------------------------------------------------------- | --------------- |
| `vecop.L2`           | `vec_distance_l2`                                          | `vector.Vector` |
| `vecop.L1`           | `vec_distance_l1`                                          | `vector.Vector` |
| `vecop.Cosine`       | `vec_distance_cosine`                                      | `vector.Vector` |
| `vecop.Hamming`      | `vec_distance_hamming(vec_bit(column), vec_bit(argument))` | `vector.Bits`   |
| `vecop.InnerProduct` | none: `ErrUnsupported` at build time                       |                 |
| `vecop.Jaccard`      | none: `ErrUnsupported` at build time                       |                 |

The query vector is a bound argument. When the column is sensitive, it shows
as `momo.RedactedArg` in hook events, on a query and on an insert, while the
database gets the real blob. Distances work in the select list, in a
comparison and in `OrderBy` (`Asc` nearest first, `Desc` farthest first).
Results are held to the plain-Go reference in `internal/vectorparity`: the
top-10 ids of cosine, L2 and L1 search over 300 vectors equal the reference
exactly. The reference is the one pgvector exact search and MariaDB are held to
for cosine, and when a PostgreSQL URL is set `TestSQLiteVectorParity` also loads
the same vectors into pgvector and requires the same ids for cosine, L2 and L1.

**Indexes.** sqlite-vec compares every row, so it has no approximate index and
`dialect.For(dialect.SQLite).Vector.Indexes` is empty, and
`schema.ValidateVectorIndex` rejects an index for SQLite with
`schema.ErrInvalidVectorIndex`. An internal check turns a request for any index
method on a dialect that lists none into `ErrUnsupported`; it becomes public
when a SQLite migration dialect needs it. SQLite itself does not parse an index method, so
nothing is accepted silently. `ExactSearch` in `vecsearch.Settings` is a no-op (every
search is exact) and any other setting is `ErrUnsupported`. The sqlite-vec
virtual tables (`vec0`, which have an index of their own) are not wrapped.

Bring up a throwaway database for your own tests with
[`momotest/sqlitetest`](testing.md#sqlite-without-a-server-sqlitetest).

## MariaDB and MySQL

MariaDB 11.8 supports float32 vectors through `mysqldriver`. MySQL 9.x
stores them but cannot compare them.

| Capability                     | MariaDB 11.8                                                  | MySQL 9.7        | MySQL 8.4              |
| ------------------------------ | ------------------------------------------------------------- | ---------------- | ---------------------- |
| Column type                    | `VECTOR(N)`                                                   | `VECTOR(N)`      | none: `ErrUnsupported` |
| `vector.Vector` write and read | yes                                                           | yes              | no                     |
| `vecop.L2`                     | `VEC_DISTANCE_EUCLIDEAN(col, VEC_FromText(?))`                | `ErrUnsupported` | `ErrUnsupported`       |
| `vecop.Cosine`                 | `VEC_DISTANCE_COSINE(col, VEC_FromText(?))`                   | `ErrUnsupported` | `ErrUnsupported`       |
| Other metrics, other types     | `ErrUnsupported` (inner product, L1, halfvec, sparsevec, bit) | `ErrUnsupported` | `ErrUnsupported`       |
| ANN index                      | `VECTOR INDEX`, approximate                                   | none             | none                   |
| `Capabilities().Vectors`       | true                                                          | false            | false                  |

MySQL Community has no distance function (the distance functions belong to
HeatWave), so a distance query on MySQL fails at build time with
`ErrUnsupported`. Momo never falls back to a scan or to a Go-side sort.

A `vector.Vector` bound on MySQL or MariaDB travels as its text form wrapped
in `VEC_FromText` (MariaDB) or `STRING_TO_VECTOR` (MySQL), on every typed bind
path: insert, update, equality and distance. Stored vectors scan back from the
server's binary float32 form. A column marked sensitive masks the vector in
hook events exactly as on PostgreSQL.

A MariaDB `VECTOR INDEX` needs a `NOT NULL` `vector(N)` column and accepts
`M` from 3 to 200 and `DISTANCE` of `euclidean` or `cosine`. It serves only
the metric it was built with, and an `ORDER BY` distance with `LIMIT` uses
it. To get the exact nearest neighbours from the same table, add `+ 0` to the
distance in a raw query, which keeps the server from using the index.
`schema.ValidateMariaDBVectorIndex` checks a declaration and
`schema.MariaDBVectorIndexClause` renders it. `momo.MySQLVectorType` gives the
column type for a server version and refuses MySQL 8.4 and MariaDB before
11.7 with a diagnostic that names the version.
