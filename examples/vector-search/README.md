# vector-search

## What this shows

Nearest-neighbour search with pgvector over items that carry an embedding.

- A `vector.Vector` field with `dim:` becomes a `vector(n)` column, and
  `MomoConfig` declares the HNSW index with its operator class and build
  parameters. The first migration also runs `CREATE EXTENSION vector`.
- `ORDER BY momo.CosineDistance(item.Embedding, query)` with a `Limit` is the
  search. Choose the distance that matches the index operator class
  (`vector_cosine_ops` for cosine, `vector_l2_ops` for `L2Distance`), or the
  index is not used. The query vector is a bound argument and a wrong dimension
  fails before any round trip.
- The distance can be selected as a column through a projection. The id is the
  last `OrderBy` key so ties return in a stable order.
- Filters: a `Where` next to the distance order. An HNSW index applies the filter
  after it returns its nearest candidates, which can leave fewer rows than the
  limit; `HNSWIterativeScan` (pgvector 0.8 or newer) keeps scanning until enough
  rows pass.
- `db.WithSearch` applies settings to one transaction only, so they never leak to
  a pooled connection and are safe behind PgBouncer: `HNSWEfSearch` trades
  latency for recall, and `ExactSearch` switches the index off for exact results.
- `Explain` shows whether the index is used. On a table this small the planner
  prefers a sequential scan, so the example turns sequential scans off for the
  transaction with `tx.SetLocal(ctx, "enable_seqscan", "off")` and checks that the
  plan names `items_embedding_hnsw`. With enough rows you leave that out. The
  explained query orders by the distance alone, without the id tie-break: the
  index delivers exactly that order, and whether the planner puts an Incremental
  Sort for the extra id key over the index scan differs between PostgreSQL versions.

The full reference is [docs/vectors.md](../../docs/vectors.md).

## Run

The role must be allowed to create the `vector` extension.

```sh
cd examples/vector-search
export DATABASE_URL=postgres://user:pass@localhost:5432/scratch
momo migrate apply
go run .
```

The test (`example_test.go`) runs the same steps in a scratch database and
checks the output.
