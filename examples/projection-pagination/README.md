# projection-pagination

## What this shows

A list screen over a table of articles whose `body` column is large.

- A projection is a struct that embeds `momo.Projection` and maps to a chosen set
  of columns. `momo.Select[models.Card]` with `From(article.Table)` and
  `Columns(...)` reads only those columns, so the database never sends `body`. The generator does not emit
  projections yet, so `models/models.go` writes their short `MomoMeta` by hand.
- Keyset pagination: `OrderBy(score desc, id asc)` and `After(score, id)`, where
  the values are those of the last row of the previous page. It stays fast and
  stable on deep pages and while rows are inserted, which `Offset` does not.
- The tie-break is what makes it correct: scores repeat, so the id is the last
  `OrderBy` key. `After` takes one value per key. For a model Momo appends the
  primary key itself; for a projection you list it. The example has ties that
  cross a page boundary.
- Fetch `size+1` rows to know whether another page exists without a `COUNT`.
- A composite index in `MomoConfig` that matches the order serves the query.
- Counting: `Count` ignores ordering and the cursor, `Exists` is `Count` with a
  limit of one, and an aggregate projection with `GroupBy` and `Having` counts
  per group.

## Run

```sh
cd examples/projection-pagination
export DATABASE_URL=postgres://user:pass@localhost:5432/scratch
momo migrate apply
go run .
```

The test (`example_test.go`) runs the same steps in a scratch database and
checks the output.
