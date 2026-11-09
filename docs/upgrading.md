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

| Old identifier            | New import path                    | New identifier         |
| ------------------------- | ---------------------------------- | ---------------------- |
| `momo.ArrayContains`      | `github.com/azrtydxb/momo/arrayop` | `arrayop.Contains`     |
| `momo.ArrayContainedBy`   | `github.com/azrtydxb/momo/arrayop` | `arrayop.ContainedBy`  |
| `momo.ArrayOverlaps`      | `github.com/azrtydxb/momo/arrayop` | `arrayop.Overlaps`     |
| `momo.AnyOf`              | `github.com/azrtydxb/momo/arrayop` | `arrayop.AnyOf`        |
| `momo.TSExpr`             | `github.com/azrtydxb/momo/fts`     | `fts.Expr`             |
| `momo.ToTSVector`         | `github.com/azrtydxb/momo/fts`     | `fts.ToVector`         |
| `momo.WebsearchToTSQuery` | `github.com/azrtydxb/momo/fts`     | `fts.WebsearchToQuery` |
| `momo.PlainToTSQuery`     | `github.com/azrtydxb/momo/fts`     | `fts.PlainToQuery`     |
| `momo.PhraseToTSQuery`    | `github.com/azrtydxb/momo/fts`     | `fts.PhraseToQuery`    |
| `momo.ToTSQuery`          | `github.com/azrtydxb/momo/fts`     | `fts.ToQuery`          |
| `momo.TSMatch`            | `github.com/azrtydxb/momo/fts`     | `fts.Match`            |
| `momo.TSRank`             | `github.com/azrtydxb/momo/fts`     | `fts.Rank`             |
| `momo.TSRankCD`           | `github.com/azrtydxb/momo/fts`     | `fts.RankCD`           |
| `momo.TSHeadline`         | `github.com/azrtydxb/momo/fts`     | `fts.Headline`         |

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
