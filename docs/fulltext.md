# Full-text search

PostgreSQL full-text search has a column type and typed expressions in Momo.
Every builder here returns `ErrUnsupported` when the statement is built for
SQLite or MySQL.

## The column

A model field of type `types.TSVector` maps to a `tsvector` column (and a
`types.TSQuery` field to `tsquery`). Both are codecs, so the generator and
`migrate` pick the column type from the Go type, diff it and read it back from
the database with no drift. The Go value is the text form PostgreSQL prints,
for example `'cat':3 'fat':2`. A nullable column is a `*types.TSVector`.

The application normally never sets this column (leave it nil on insert and
update: the trigger overwrites it). Keep it current with a
trigger declared in `MomoConfig`, and index it with GIN:

<!-- docscheck:fragment -->

```go
type Article struct {
	momo.Model `momo:"table:kb_articles"`
	ID         int64            `momo:"pk,identity"`
	Title      string
	Body       string
	SearchTsv  *types.TSVector
	Embedding  vector.Vector `momo:"dim:1536"`
}

func (a *Article) MomoConfig(c *momo.Config) {
	c.Index("kb_articles_search_gin", &a.SearchTsv).Method("gin")
	c.Function(schema.Function{
		Name: "kb_articles_tsv", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE",
		Body: "BEGIN NEW.search_tsv := setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') || " +
			"to_tsvector('english', coalesce(NEW.body, '')); RETURN NEW; END;",
	})
	c.Trigger(schema.Trigger{
		Table: "kb_articles", Name: "kb_articles_tsv_trg", Timing: "BEFORE",
		Events: []string{"INSERT", "UPDATE"}, ForEachRow: true, Function: "kb_articles_tsv()",
	})
}
```

## Expressions

The text search configuration (`english`, `simple`, ...) is a string. It is
bound as a parameter and cast to regconfig, so it cannot carry SQL; an empty
one fails the build with `ErrValidation`.

| Builder                   | SQL                                            | Result type              |
| ------------------------- | ---------------------------------------------- | ------------------------ |
| `momo.TSMatch`            | `vec @@ query`                                 | `Predicate`              |
| `momo.ToTSVector`         | `to_tsvector(config, text)`                    | `TSExpr[types.TSVector]` |
| `momo.WebsearchToTSQuery` | `websearch_to_tsquery(config, text)`           | `TSExpr[types.TSQuery]`  |
| `momo.PlainToTSQuery`     | `plainto_tsquery(config, text)`                | `TSExpr[types.TSQuery]`  |
| `momo.PhraseToTSQuery`    | `phraseto_tsquery(config, text)`               | `TSExpr[types.TSQuery]`  |
| `momo.ToTSQuery`          | `to_tsquery(config, text)`                     | `TSExpr[types.TSQuery]`  |
| `momo.TSRank`             | `ts_rank(vec, query [, normalization])`        | `TSExpr[float64]`        |
| `momo.TSRankCD`           | `ts_rank_cd(vec, query [, normalization])`     | `TSExpr[float64]`        |
| `momo.TSHeadline`         | `ts_headline(config, text, query [, options])` | `TSExpr[string]`         |

The query text, the configuration and the headline options are always bound
parameters. The normalization argument of the rank functions is the optional
PostgreSQL flag mask (0 to 63) and renders as a literal. `WebsearchToTSQuery`
accepts any user input without raising a syntax error, so it is the builder for
a search box; `ToTSQuery` expects tsquery syntax and a malformed query is a
database error.

A `TSExpr` has `As`, `Asc`, `Desc` and the comparison methods of any scalar
expression, so it works in Where, OrderBy and Columns:

<!-- docscheck:fragment -->

```go
q := momo.WebsearchToTSQuery("english", userInput)
rank := momo.TSRankCD(articleSearch, q)

hits, err := momo.Select[Hit](db).
	From(momo.TableRef{Name: "kb_articles"}).
	Columns(articleID, rank.As("rank"),
		momo.TSHeadline("english", articleBody, q, "StartSel=<b>, StopSel=</b>, MaxFragments=2")).
	Where(momo.TSMatch(articleSearch, q)).
	OrderBy(rank.Desc()).
	Limit(20).
	All(ctx)
```

Here `articleSearch` is a `Column[types.TSVector]` and `articleBody` a
`Column[string]`. Without a stored column, search a text column directly with
`momo.ToTSVector` over `articleBody` as the first operand of `TSMatch` (an
expression index on the same expression keeps it fast).

Every use of a call binds its own parameters (the configuration and the query
text), so the same expression written in the select list and again in GROUP BY
gets different placeholders, and PostgreSQL rejects the statement as an
ungrouped column. The same applies to SELECT DISTINCT with an ORDER BY on the
call. Group by a stored tsvector instead, or compute the call once in a
subquery or CTE and refer to its alias. A golden test pins the placeholders.

Arguments bound by these calls are masked in hook events (shown as
`RedactedArg`) when an operand reads a column marked sensitive; the driver
still receives the real values.

## Hybrid ranking: vector plus text

Momo has no built-in fusion step (reciprocal rank fusion is not built). Combine
the two signals in one score with the typed arithmetic helpers. The cosine
distance is lower for better matches and the text rank is higher, so subtract a
weighted rank from the distance and order ascending:

<!-- docscheck:fragment -->

```go
dist := momo.CosineDistance(articleEmb, queryEmbedding) // vector.Vector
text := momo.TSRankCD(articleSearch, momo.WebsearchToTSQuery("english", userInput))
score := momo.SubExpr[float64](dist, momo.Mul[float64](text, 0.5))

hits, err := momo.Select[Hit](db).
	From(momo.TableRef{Name: "kb_articles"}).
	Columns(articleID, score.As("score"), articleTitle).
	OrderBy(score.Asc()).
	Limit(20).
	All(ctx)
```

This scores every row. To restrict the candidate set to text matches, add
a Where on `TSMatch` over `articleSearch` and the same query, which lets the GIN
index prune the rows before the vector distance is computed. The
`TestFullTextHybridRanking` integration test runs this pattern.

## Scanning

pgx returns tsvector and tsquery values as text, which `types.TSVector` and
`types.TSQuery` scan. `ts_rank` and `ts_rank_cd` return real; they scan into
`float64` fields.
