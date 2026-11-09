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

| Old identifier          | New import path                    | New identifier            |
| ----------------------- | ---------------------------------- | ------------------------- |
| `momo.ArrayContains`    | `github.com/azrtydxb/momo/arrayop` | `arrayop.Contains`        |
| `momo.ArrayContainedBy` | `github.com/azrtydxb/momo/arrayop` | `arrayop.ContainedBy`     |
| `momo.ArrayOverlaps`    | `github.com/azrtydxb/momo/arrayop` | `arrayop.Overlaps`        |
| `momo.AnyOf`            | `github.com/azrtydxb/momo/arrayop` | `arrayop.AnyOf`           |
| `momo.ExtractField`     | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractField`     |
| `momo.ExtractExpr`      | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractExpr`      |
| `momo.Extract`          | `github.com/azrtydxb/momo/timeop`  | `timeop.Extract`          |
| `momo.ExtractYear`      | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractYear`      |
| `momo.ExtractMonth`     | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractMonth`     |
| `momo.ExtractDay`       | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractDay`       |
| `momo.ExtractHour`      | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractHour`      |
| `momo.ExtractMinute`    | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractMinute`    |
| `momo.ExtractSecond`    | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractSecond`    |
| `momo.ExtractQuarter`   | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractQuarter`   |
| `momo.ExtractDow`       | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractDow`       |
| `momo.ExtractISODow`    | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractISODow`    |
| `momo.ExtractDoy`       | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractDoy`       |
| `momo.ExtractEpoch`     | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractEpoch`     |
| `momo.TimeExpr`         | `github.com/azrtydxb/momo/timeop`  | `timeop.Expr`             |
| `momo.AtTimeZone`       | `github.com/azrtydxb/momo/timeop`  | `timeop.AtTimeZone`       |
| `momo.AtTimeZoneExpr`   | `github.com/azrtydxb/momo/timeop`  | `timeop.AtTimeZoneExpr`   |
| `momo.IntervalUnit`     | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalUnit`     |
| `momo.Interval`         | `github.com/azrtydxb/momo/timeop`  | `timeop.Interval`         |
| `momo.IntervalOf`       | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalOf`       |
| `momo.IntervalExpr`     | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalExpr`     |
| `momo.IntervalSecond`   | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalSecond`   |
| `momo.IntervalMinute`   | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalMinute`   |
| `momo.IntervalHour`     | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalHour`     |
| `momo.IntervalDay`      | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalDay`      |
| `momo.IntervalWeek`     | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalWeek`     |
| `momo.IntervalMonth`    | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalMonth`    |
| `momo.IntervalYear`     | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalYear`     |
| `momo.AddInterval`      | `github.com/azrtydxb/momo/timeop`  | `timeop.AddInterval`      |
| `momo.SubInterval`      | `github.com/azrtydxb/momo/timeop`  | `timeop.SubInterval`      |
| `momo.Now`              | `github.com/azrtydxb/momo/timeop`  | `timeop.Now`              |
| `momo.CurrentTimestamp` | `github.com/azrtydxb/momo/timeop`  | `timeop.CurrentTimestamp` |
| `momo.TruncUnit`        | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncUnit`        |
| `momo.DateTrunc`        | `github.com/azrtydxb/momo/timeop`  | `timeop.Trunc`            |
| `momo.TruncSecond`      | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncSecond`      |
| `momo.TruncMinute`      | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncMinute`      |
| `momo.TruncHour`        | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncHour`        |
| `momo.TruncDay`         | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncDay`         |
| `momo.TruncWeek`        | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncWeek`        |
| `momo.TruncMonth`       | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncMonth`       |
| `momo.TruncQuarter`     | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncQuarter`     |
| `momo.TruncYear`        | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncYear`        |

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
