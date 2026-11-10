# Projections

A projection is a result type that is not a table: a subset of a table's columns, an aggregate, or a computed column such as a vector distance. You select it with `momo.Select[T]`, name the source with `From` and the select list with `Columns`. A projection is read only: it cannot be inserted, updated or deleted.

## Declaring one

Embed `momo.Projection` by value in a struct. Every exported field is one column of the select list, in declaration order.

<!-- docscheck:fragment -->

```go
type Card struct {
	momo.Projection
	ID    int64
	Title string
	Score int64 `momo:"column:points"`
}
```

- The column name of a field follows the naming rule of the project (`naming:` in `momo.yaml`), as for model fields. A `momo:"column:..."` tag overrides it. The name is a label: the select list is positional and not aliased.
- Unexported fields and fields tagged `momo:"-"` are skipped. Relation tags are an error on a projection.
- A projection has no table, no primary key, no indexes and no `MomoConfig`. The generator does not add it to the schema, so it never appears in a migration.
- A field type must be one the generator maps (see [the type mapping](TYPE_MAPPING.md)). An unsupported type fails `momo generate` with a message that names the struct and the field, for example `projection Odd: field Value: Go type complex128 has no SQL mapping`.

## The generated metadata

`momo generate` writes, for each projection, a `MomoMeta` method and a `momo.Meta` value into the `momo_gen.go` of the package that declares it. `Columns`, `Ptr` and `Value` cover every field in declaration order. `Identity`, `Version`, `SoftDelete`, `CreatedAt` and `UpdatedAt` are all `-1`, because a projection has none of those columns. Do not write this by hand. A package that holds only projections gets a `momo_gen.go` too.

## Reading

<!-- docscheck:fragment -->

```go
cards, err := momo.Select[models.Card](q).
	From(article.Table).
	Columns(article.ID, article.Title, article.Score).
	OrderBy(article.Score.Desc(), article.ID.Asc()).
	All(ctx)
```

`From` takes the generated Table of a model package, an aliased table (`article.Table.As("a")`), a CTE or a subquery. `Columns` must list as many expressions as the struct has fields, in field order. Without `Columns` the query fails with `ErrValidation`. Computed expressions (a distance, `momo.CountAll()`) are valid entries.

Keyset pagination over a projection lists the unique tie-break key in `OrderBy` yourself, since the metadata has no primary key. See [projection-pagination](../examples/projection-pagination/).

## Tuple rows: no struct per shape

For an aggregate, a join or a few columns that you read in one place, skip the struct and the generated metadata: `momo.Row1` to `momo.Row10` are generic projections with fields `C1` to `Cn`. `Columns` is scanned positionally, so expression `i` fills `Ci`.

<!-- docscheck:fragment -->

```go
// a count per key
rows, err := momo.Select[momo.Row2[string, int64]](db).
	From(license.Table).
	Columns(license.Key, momo.CountAll()).
	GroupBy(license.Key).
	All(ctx)
for _, r := range rows {
	fmt.Println(r.C1, r.C2) // key, count
}

// a single aggregate
total, err := momo.Select[momo.Row1[int64]](db).From(license.Table).Columns(momo.CountAll()).One(ctx)
// total.C1
```

- A column that can be NULL takes a pointer type: a `LeftJoin` side or an aggregate over no rows. With a `Row3` of `string, *int64, *string`, the columns `a.Title, b.ID, b.Title` fill `C1` to `C3`, and `C2` and `C3` stay nil when the join finds no row.
- Subquery columns, `Union`, `FromSubquery`, With and `WithRecursive` work as for any projection. The Go types of the columns are yours to match; they are not checked against the SQL.
- A mismatch between the number of `Columns` and the arity of the row is `ErrValidation`, reported before any statement runs. So is omitting `Columns`.
- A tuple row is a projection with no table: `Insert`, `Update`, `Delete`, `Upsert`, `SoftDelete` and `ForceDelete` refuse it with `ErrValidation`. It has no soft-delete column of its own, and the filter on the `From` table applies as described below, so a `Row1[int64]` with `CountAll` over `article.Table` counts live rows, and `WithDeleted` counts all.
- `After` works when `OrderBy` lists a unique key, since a tuple has no primary key to break ties.
- The metadata is built once per instantiation and shared; looking it up does not allocate.

When the result has meaning worth a name, or more than ten columns, declare a projection struct instead and let `momo generate` write its metadata.

## Soft-deleted rows

When `From` is the generated Table of a model that has a soft-delete column (timestamp, unix or bool), a projection excludes soft-deleted rows by default, exactly as a model query does. `WithDeleted()` includes them and `OnlyDeleted()` returns only them. Do not add the `deleted_at` predicate by hand.

The filter applies to the `From` table whatever else the query does: with `Join` or `LeftJoin` the `From` table is still filtered, and the joined tables are not (filter them in the `ON` clause or a `Where`, as for a model). To alias the table keep the generated one and call `As`: `article.Table.As("a")` keeps the soft-delete column, so the filter is qualified with the alias.

The filter is not added when:

- `From` is a subquery or CTE: filter inside it;
- `From` is a hand-built `momo.TableRef{Name: ..., Alias: ...}`: it carries no soft-delete column. Start from the generated `article.Table` instead, or add the predicate yourself, for example `Where(article.DeletedAt.IsNull())`.

## Upgrading from hand-written metadata

Before the generator emitted projections, the guides told you to write `MomoMeta` and a `momo.Meta` value for each projection by hand. After you upgrade, `momo generate` writes them too, and the package no longer compiles (`MomoMeta redeclared`). Delete the hand-written `MomoMeta` method and its `Meta` variable for every struct that embeds `momo.Projection`, then run `momo generate`. Check that the field order and `momo:"column:..."` tags give the column names you had. Remove any `Where(<table>.DeletedAt.IsNull())` you added to projection queries; keeping it is harmless but redundant.

## Examples

- [projection-pagination](../examples/projection-pagination/): `Card` and an aggregate projection, keyset pagination, counting.
- [vector-search](../examples/vector-search/): `Hit`, a title together with its distance.
