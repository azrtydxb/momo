# Upgrading

Momo is pre-v1 and makes no compatibility promise between releases; see the
[stability policy](supported-versions.md#stability-policy). Read this page and
the `CHANGELOG.md` entry of a release before you upgrade, and expect that
later releases can break more.

## Upgrading to 0.4.0

Release 0.4.0 moves feature-specific helpers out of the root package `momo`
into subpackages. Every move is a source break: the old identifier is gone and
the new one lives under a new import path.

Why there is no alias: a Go method lives in the package of its receiver type,
and the subpackages import `momo`, so `momo` cannot import them back to keep
the old names. A move therefore deletes the old identifier outright.

### Identifier table

One row per moved identifier. When a method became a function, the old name is
the method and the new name is the function, which takes the former receiver
as an argument.

| Old identifier            | New import path                    | New identifier            |
| ------------------------- | ---------------------------------- | ------------------------- |
| `momo.ArrayContains`      | `github.com/azrtydxb/momo/arrayop` | `arrayop.Contains`        |
| `momo.ArrayContainedBy`   | `github.com/azrtydxb/momo/arrayop` | `arrayop.ContainedBy`     |
| `momo.ArrayOverlaps`      | `github.com/azrtydxb/momo/arrayop` | `arrayop.Overlaps`        |
| `momo.AnyOf`              | `github.com/azrtydxb/momo/arrayop` | `arrayop.AnyOf`           |
| `momo.TSExpr`             | `github.com/azrtydxb/momo/fts`     | `fts.Expr`                |
| `momo.ToTSVector`         | `github.com/azrtydxb/momo/fts`     | `fts.ToVector`            |
| `momo.WebsearchToTSQuery` | `github.com/azrtydxb/momo/fts`     | `fts.WebsearchToQuery`    |
| `momo.PlainToTSQuery`     | `github.com/azrtydxb/momo/fts`     | `fts.PlainToQuery`        |
| `momo.PhraseToTSQuery`    | `github.com/azrtydxb/momo/fts`     | `fts.PhraseToQuery`       |
| `momo.ToTSQuery`          | `github.com/azrtydxb/momo/fts`     | `fts.ToQuery`             |
| `momo.TSMatch`            | `github.com/azrtydxb/momo/fts`     | `fts.Match`               |
| `momo.TSRank`             | `github.com/azrtydxb/momo/fts`     | `fts.Rank`                |
| `momo.TSRankCD`           | `github.com/azrtydxb/momo/fts`     | `fts.RankCD`              |
| `momo.TSHeadline`         | `github.com/azrtydxb/momo/fts`     | `fts.Headline`            |
| `momo.Substr`             | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Substr`            |
| `momo.SubstrFrom`         | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.SubstrFrom`        |
| `momo.Lower`              | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Lower`             |
| `momo.Upper`              | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Upper`             |
| `momo.Length`             | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Length`            |
| `momo.LengthExpr`         | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.LengthExpr`        |
| `momo.CurrentSchema`      | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.CurrentSchema`     |
| `momo.CurrentSchemaExpr`  | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.CurrentSchemaExpr` |
| `momo.Case`               | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Case`              |
| `momo.CaseExpr`           | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.CaseExpr`          |
| `momo.Numeric`            | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Numeric`           |
| `momo.ArithExpr`          | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.ArithExpr`         |
| `momo.Add`                | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Add`               |
| `momo.AddExpr`            | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.AddExpr`           |
| `momo.Sub`                | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Sub`               |
| `momo.SubExpr`            | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.SubExpr`           |
| `momo.Mul`                | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Mul`               |
| `momo.MulExpr`            | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.MulExpr`           |
| `momo.CoalesceExpr`       | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.CoalesceExpr`      |
| `momo.Cast`               | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Cast`              |
| `momo.CastExpr`           | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.CastExpr`          |
| `momo.Concat`             | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.Concat`            |
| `momo.ConcatExpr`         | `github.com/azrtydxb/momo/sqlfn`   | `sqlfn.ConcatExpr`        |
| `momo.JSONText`           | `github.com/azrtydxb/momo/jsonop`  | `jsonop.Text`             |
| `momo.JSONPath`           | `github.com/azrtydxb/momo/jsonop`  | `jsonop.Path`             |
| `momo.JSONContains`       | `github.com/azrtydxb/momo/jsonop`  | `jsonop.Contains`         |
| `momo.JSONHasKey`         | `github.com/azrtydxb/momo/jsonop`  | `jsonop.HasKey`           |
| `momo.JSONTextExpr`       | `github.com/azrtydxb/momo/jsonop`  | `jsonop.TextExpr`         |
| `momo.AggExpr`            | `github.com/azrtydxb/momo/agg`     | `agg.Call`                |
| `momo.Aggregate`          | `github.com/azrtydxb/momo/agg`     | `agg.Func`                |
| `momo.StringAgg`          | `github.com/azrtydxb/momo/agg`     | `agg.String`              |
| `momo.ArrayAgg`           | `github.com/azrtydxb/momo/agg`     | `agg.Array`               |
| `momo.JSONBAgg`           | `github.com/azrtydxb/momo/agg`     | `agg.JSONB`               |
| `momo.PercentileCont`     | `github.com/azrtydxb/momo/agg`     | `agg.PercentileCont`      |
| `momo.PercentileDisc`     | `github.com/azrtydxb/momo/agg`     | `agg.PercentileDisc`      |
| `momo.JSONPair`           | `github.com/azrtydxb/momo/agg`     | `agg.Pair`                |
| `momo.JSONField`          | `github.com/azrtydxb/momo/agg`     | `agg.Field`               |
| `momo.JSONObjectExpr`     | `github.com/azrtydxb/momo/agg`     | `agg.ObjectExpr`          |
| `momo.JSONBBuildObject`   | `github.com/azrtydxb/momo/agg`     | `agg.BuildObject`         |
| `momo.ToJSONExpr`         | `github.com/azrtydxb/momo/agg`     | `agg.JSONExpr`            |
| `momo.ToJSONB`            | `github.com/azrtydxb/momo/agg`     | `agg.ToJSONB`             |
| `momo.EmptyJSONArray`     | `github.com/azrtydxb/momo/agg`     | `agg.EmptyJSONArray`      |

### Rewrite recipe

1. Find each old identifier of the table in your code (`grep` for `momo.` plus
   the old name, or let the compiler list them).
2. Replace it with the new identifier and import the new package.
3. Run `gofmt` and build.

A row never changes the SQL a query produces; only the Go name and import path
change.

One type gained methods: `sqlfn.CurrentSchemaExpr` now has the comparison and
ordering methods (`Eq`, `In`, `Asc`, ...) the other function results have, so
`sqlfn.CurrentSchema().Eq("app")` compiles where `momo.CurrentSchema()` had
no such method.

### What did not move

These stay in `momo`, with the reason:

- `Func` and `FuncExpr`: the generic SQL function escape hatch, which the new
  packages build on.
- `Coalesce`, `Value`, `SQL` and `TypedSQL`: bound-value and raw-fragment
  building blocks of the expression core. `sqlfn.CoalesceExpr` takes expression
  arguments instead.
- The vector column types (`VectorColumn` and its variants): generated code
  references them.
- The hook interfaces (`Hook`, `QueryEvent`, `RelationHook`, `RelationEvent`):
  `DB` stores the hooks and calls them.
- The basic aggregates (`Agg[T]`, `CountAll`, `Sum`, `Avg`, `Min`, `Max`):
  `Having` and select lists are typed on `Agg`.
- `Tx.SetLocal`: a method of `Tx`, and other helpers call it.
