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
| `momo.ExtractField`       | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractField`     |
| `momo.ExtractExpr`        | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractExpr`      |
| `momo.Extract`            | `github.com/azrtydxb/momo/timeop`  | `timeop.Extract`          |
| `momo.ExtractYear`        | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractYear`      |
| `momo.ExtractMonth`       | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractMonth`     |
| `momo.ExtractDay`         | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractDay`       |
| `momo.ExtractHour`        | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractHour`      |
| `momo.ExtractMinute`      | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractMinute`    |
| `momo.ExtractSecond`      | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractSecond`    |
| `momo.ExtractQuarter`     | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractQuarter`   |
| `momo.ExtractDow`         | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractDow`       |
| `momo.ExtractISODow`      | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractISODow`    |
| `momo.ExtractDoy`         | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractDoy`       |
| `momo.ExtractEpoch`       | `github.com/azrtydxb/momo/timeop`  | `timeop.ExtractEpoch`     |
| `momo.TimeExpr`           | `github.com/azrtydxb/momo/timeop`  | `timeop.Expr`             |
| `momo.AtTimeZone`         | `github.com/azrtydxb/momo/timeop`  | `timeop.AtTimeZone`       |
| `momo.AtTimeZoneExpr`     | `github.com/azrtydxb/momo/timeop`  | `timeop.AtTimeZoneExpr`   |
| `momo.IntervalUnit`       | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalUnit`     |
| `momo.Interval`           | `github.com/azrtydxb/momo/timeop`  | `timeop.Interval`         |
| `momo.IntervalOf`         | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalOf`       |
| `momo.IntervalExpr`       | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalExpr`     |
| `momo.IntervalSecond`     | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalSecond`   |
| `momo.IntervalMinute`     | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalMinute`   |
| `momo.IntervalHour`       | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalHour`     |
| `momo.IntervalDay`        | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalDay`      |
| `momo.IntervalWeek`       | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalWeek`     |
| `momo.IntervalMonth`      | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalMonth`    |
| `momo.IntervalYear`       | `github.com/azrtydxb/momo/timeop`  | `timeop.IntervalYear`     |
| `momo.AddInterval`        | `github.com/azrtydxb/momo/timeop`  | `timeop.AddInterval`      |
| `momo.SubInterval`        | `github.com/azrtydxb/momo/timeop`  | `timeop.SubInterval`      |
| `momo.Now`                | `github.com/azrtydxb/momo/timeop`  | `timeop.Now`              |
| `momo.CurrentTimestamp`   | `github.com/azrtydxb/momo/timeop`  | `timeop.CurrentTimestamp` |
| `momo.TruncUnit`          | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncUnit`        |
| `momo.DateTrunc`          | `github.com/azrtydxb/momo/timeop`  | `timeop.Trunc`            |
| `momo.TruncSecond`        | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncSecond`      |
| `momo.TruncMinute`        | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncMinute`      |
| `momo.TruncHour`          | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncHour`        |
| `momo.TruncDay`           | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncDay`         |
| `momo.TruncWeek`          | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncWeek`        |
| `momo.TruncMonth`         | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncMonth`       |
| `momo.TruncQuarter`       | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncQuarter`     |
| `momo.TruncYear`          | `github.com/azrtydxb/momo/timeop`  | `timeop.TruncYear`        |

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
