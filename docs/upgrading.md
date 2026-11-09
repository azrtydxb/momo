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

| Old identifier          | New import path                    | New identifier        |
| ----------------------- | ---------------------------------- | --------------------- |
| `momo.ArrayContains`    | `github.com/azrtydxb/momo/arrayop` | `arrayop.Contains`    |
| `momo.ArrayContainedBy` | `github.com/azrtydxb/momo/arrayop` | `arrayop.ContainedBy` |
| `momo.ArrayOverlaps`    | `github.com/azrtydxb/momo/arrayop` | `arrayop.Overlaps`    |
| `momo.AnyOf`            | `github.com/azrtydxb/momo/arrayop` | `arrayop.AnyOf`       |
| `momo.AggExpr`          | `github.com/azrtydxb/momo/agg`     | `agg.Call`            |
| `momo.Aggregate`        | `github.com/azrtydxb/momo/agg`     | `agg.Func`            |
| `momo.StringAgg`        | `github.com/azrtydxb/momo/agg`     | `agg.String`          |
| `momo.ArrayAgg`         | `github.com/azrtydxb/momo/agg`     | `agg.Array`           |
| `momo.JSONBAgg`         | `github.com/azrtydxb/momo/agg`     | `agg.JSONB`           |
| `momo.PercentileCont`   | `github.com/azrtydxb/momo/agg`     | `agg.PercentileCont`  |
| `momo.PercentileDisc`   | `github.com/azrtydxb/momo/agg`     | `agg.PercentileDisc`  |
| `momo.JSONPair`         | `github.com/azrtydxb/momo/agg`     | `agg.Pair`            |
| `momo.JSONField`        | `github.com/azrtydxb/momo/agg`     | `agg.Field`           |
| `momo.JSONObjectExpr`   | `github.com/azrtydxb/momo/agg`     | `agg.ObjectExpr`      |
| `momo.JSONBBuildObject` | `github.com/azrtydxb/momo/agg`     | `agg.BuildObject`     |
| `momo.ToJSONExpr`       | `github.com/azrtydxb/momo/agg`     | `agg.JSONExpr`        |
| `momo.ToJSONB`          | `github.com/azrtydxb/momo/agg`     | `agg.ToJSONB`         |
| `momo.EmptyJSONArray`   | `github.com/azrtydxb/momo/agg`     | `agg.EmptyJSONArray`  |

### Rewrite recipe

1. Find each old identifier of the table in your code (`grep` for `momo.` plus
   the old name, or let the compiler list them).
2. Replace it with the new identifier and import the new package.
3. Run `gofmt` and build.

A row never changes the SQL a query produces; only the Go name and import path
change.

### What did not move

These stay in `momo`, with the reason:

- `Func` and `FuncExpr`: the generic SQL function escape hatch, which the new
  packages build on.
- The vector column types (`VectorColumn` and its variants): generated code
  references them.
- The hook interfaces (`Hook`, `QueryEvent`, `RelationHook`, `RelationEvent`):
  `DB` stores the hooks and calls them.
- The basic aggregates (`Agg[T]`, `CountAll`, `Sum`, `Avg`, `Min`, `Max`):
  `Having` and select lists are typed on `Agg`.
- `Tx.SetLocal`: a method of `Tx`, and other helpers call it.
