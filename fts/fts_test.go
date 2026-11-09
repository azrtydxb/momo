// SPDX-License-Identifier: Apache-2.0

package fts_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/fts"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/types"
)

const sensitiveValue = "hunter2-sensitive"

type tallyQuery = momo.SelectQuery[exprtest.Tally]

var (
	tsv   = momo.NewColumn[types.TSVector]("articles", "search_tsv", 0)
	title = momo.NewColumn[string]("articles", "title", 0)
)

// maskCase runs one statement on PostgreSQL and checks the hook arguments
// position by position: every driver argument holding the secret, and every
// value in also (the other values bound with it), shows as RedactedArg in the
// last hook event; every other argument is unchanged. wantSecret is the exact
// number of arguments holding the secret.
func maskCase(t *testing.T, name string, wantSecret int, also []any, run func(ctx context.Context, db *momo.DB) error) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		rec := &obstest.Recorder{}
		drv := fakedriver.New(dialect.Postgres)
		db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
		if err := run(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		calls, evs := drv.Calls(), rec.Events()
		if len(calls) == 0 || len(evs) == 0 {
			t.Fatalf("calls %d, events %d", len(calls), len(evs))
		}
		sent, seen := calls[len(calls)-1].Args, evs[len(evs)-1].Args
		if len(sent) != len(seen) {
			t.Fatalf("driver got %d args, hook saw %d", len(sent), len(seen))
		}
		secrets, extra := 0, 0
		for i := range sent {
			switch s, _ := sent[i].(string); {
			case strings.Contains(s, sensitiveValue):
				secrets++
			case slices.Contains(also, sent[i]):
				extra++
			default:
				if seen[i] != sent[i] {
					t.Errorf("arg %d: hook saw %v, driver got %v", i, seen[i], sent[i])
				}
				continue
			}
			if seen[i] != momo.RedactedArg {
				t.Errorf("arg %d: hook saw %v, want the placeholder (driver args %v)", i, seen[i], sent)
			}
		}
		if secrets != wantSecret || extra != len(also) {
			t.Errorf("masked %d secrets and %d sibling values, want %d and %d (driver args %v)", secrets, extra, wantSecret, len(also), sent)
		}
	})
}

// Catches: a full-text call that binds its configuration, query text or
// headline options on the plain arg path, leaking values compared with a
// sensitive column into hook events, or masking arguments of a plain column.
func TestFullTextHooksSeeMaskedArgs(t *testing.T) {
	secretTsv := momo.NewColumn[types.TSVector]("documents", "secret_tsv", momo.FlagSensitive)
	plainTsv := momo.NewColumn[types.TSVector]("documents", "search_tsv", 0)
	sel := func(p momo.Predicate) func(context.Context, *momo.DB) error {
		return func(ctx context.Context, db *momo.DB) error {
			_, err := momo.Select[suite.Document](db).Where(p).All(ctx)
			return err
		}
	}
	web := fts.WebsearchToQuery

	maskCase(t, "match sensitive column", 1, []any{"english"},
		sel(fts.Match(secretTsv, web("english", sensitiveValue))))
	maskCase(t, "match plain column", 0, nil,
		sel(fts.Match(plainTsv, web("english", "visible"))))
	maskCase(t, "to_tsvector over a sensitive column", 1, []any{"english", "simple"},
		sel(fts.Match(fts.ToVector("english", suite.DocSecret), web("simple", sensitiveValue))))
	maskCase(t, "rank over a sensitive column", 1, []any{"english", 0.1},
		sel(fts.RankCD(secretTsv, web("english", sensitiveValue)).Gt(0.1)))
	maskCase(t, "rank over a plain column", 0, nil,
		sel(fts.Rank(plainTsv, web("english", "visible")).Gt(0.1)))
	maskCase(t, "headline over a sensitive column", 0, []any{"english", "simple", "x", "StartSel=<b>"},
		func(ctx context.Context, db *momo.DB) error {
			h := fts.Headline("english", suite.DocSecret, web("simple", "x"), "StartSel=<b>")
			_, err := momo.Select[suite.Document](db).OrderBy(h.Asc()).All(ctx)
			return err
		})
}

// compile renders the select-list expression e, optionally ordered or grouped,
// as a statement over the articles table.
func compile(e momo.Expr, mod func(q tallyQuery) tallyQuery) (momo.Compiled, error) {
	q := momo.Select[exprtest.Tally](exprtest.DB(dialect.Postgres)).From(momo.TableRef{Name: "articles"}).Columns(e)
	if mod != nil {
		q = mod(q)
	}
	return q.SQL()
}

// Catches: every use of a full-text call must bind its own parameters, so the
// same expression in the select list and in GROUP BY gets different
// placeholders, which PostgreSQL rejects as an ungrouped column. Group by a
// stored tsvector, or compute the call in a subquery or CTE.
func TestFullTextBindsAreNotGroupable(t *testing.T) {
	rank := fts.Rank(tsv, fts.WebsearchToQuery("english", "red fox"))
	got, err := compile(rank, func(q tallyQuery) tallyQuery {
		return q.GroupBy(rank)
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT ts_rank("articles"."search_tsv", websearch_to_tsquery(CAST($1 AS regconfig), $2)) FROM "articles" GROUP BY ts_rank("articles"."search_tsv", websearch_to_tsquery(CAST($3 AS regconfig), $4))`
	if got.SQL != want {
		t.Errorf("got %s\nwant %s", got.SQL, want)
	}
}

// Catches: a wrong function name or argument order, a configuration bound
// without its regconfig cast, a value interpolated instead of bound, a
// normalization that is not a literal, and options that are not bound.
func TestFullTextGolden(t *testing.T) {
	q := fts.WebsearchToQuery("english", "red fox")
	order := func(o momo.Order) func(tallyQuery) tallyQuery {
		return func(s tallyQuery) tallyQuery { return s.OrderBy(o) }
	}
	where := func(p momo.Predicate) func(tallyQuery) tallyQuery {
		return func(s tallyQuery) tallyQuery { return s.Where(p) }
	}

	cases := []struct {
		name string
		e    momo.Expr
		mod  func(tallyQuery) tallyQuery
		want string
		args []any
	}{
		{"match with websearch", momo.CountAll(), where(fts.Match(tsv, q)),
			`SELECT COUNT(*) FROM "articles" WHERE "articles"."search_tsv" @@ websearch_to_tsquery(CAST($1 AS regconfig), $2)`,
			[]any{"english", "red fox"}},
		{"plain phrase and to_tsquery", momo.CountAll(), where(momo.Or(
			fts.Match(tsv, fts.PlainToQuery("simple", "a b")),
			fts.Match(tsv, fts.PhraseToQuery("simple", "a b")),
			fts.Match(tsv, fts.ToQuery("simple", "a & b:*")),
		)),
			`WHERE ("articles"."search_tsv" @@ plainto_tsquery(CAST($1 AS regconfig), $2) OR "articles"."search_tsv" @@ phraseto_tsquery(CAST($3 AS regconfig), $4) OR "articles"."search_tsv" @@ to_tsquery(CAST($5 AS regconfig), $6))`,
			[]any{"simple", "a b", "simple", "a b", "simple", "a & b:*"}},
		{"to_tsvector on a text column", momo.CountAll(), where(fts.Match(fts.ToVector("english", title), q)),
			`WHERE to_tsvector(CAST($1 AS regconfig), "articles"."title") @@ websearch_to_tsquery(CAST($2 AS regconfig), $3)`,
			[]any{"english", "english", "red fox"}},
		{"rank in order by, descending", title, order(fts.RankCD(tsv, q).Desc()),
			`ORDER BY ts_rank_cd("articles"."search_tsv", websearch_to_tsquery(CAST($1 AS regconfig), $2)) DESC`,
			[]any{"english", "red fox"}},
		{"rank with normalization in the select list", fts.Rank(tsv, q, 33).As("rank"), nil,
			`SELECT ts_rank("articles"."search_tsv", websearch_to_tsquery(CAST($1 AS regconfig), $2), 33) AS "rank" FROM`,
			[]any{"english", "red fox"}},
		{"headline with options", fts.Headline("english", title, q, "StartSel=<b>, StopSel=</b>"), nil,
			`ts_headline(CAST($1 AS regconfig), "articles"."title", websearch_to_tsquery(CAST($2 AS regconfig), $3), $4)`,
			[]any{"english", "english", "red fox", "StartSel=<b>, StopSel=</b>"}},
		{"headline without options", fts.Headline("english", title, q, ""), nil,
			`SELECT ts_headline(CAST($1 AS regconfig), "articles"."title", websearch_to_tsquery(CAST($2 AS regconfig), $3)) FROM`,
			[]any{"english", "english", "red fox"}},
	}
	for _, c := range cases {
		got, err := compile(c.e, c.mod)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !strings.Contains(got.SQL, c.want) {
			t.Errorf("%s:\n got %s\nwant to contain %s", c.name, got.SQL, c.want)
		}
		if !reflect.DeepEqual(got.Args, c.args) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.args)
		}
	}
}

// Catches: a full-text call rendered on a dialect without it, and a bad
// configuration or normalization that reaches the database instead of failing
// at build time with a typed error.
func TestFullTextUnsupportedAndInvalid(t *testing.T) {
	q := fts.WebsearchToQuery("english", "x")
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if _, err := exprtest.Compile(d, fts.Match(tsv, q)); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("Match on %s = %v, want ErrUnsupported", d, err)
		}
		if _, err := exprtest.Compile(d, fts.RankCD(tsv, q).Gt(0.1)); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("RankCD on %s = %v, want ErrUnsupported", d, err)
		}
	}
	bad := map[string]momo.Predicate{
		"empty config":      fts.Match(tsv, fts.WebsearchToQuery("", "x")),
		"empty vec config":  fts.Match(fts.ToVector("", title), q),
		"negative norm":     fts.RankCD(tsv, q, -1).Gt(0),
		"norm out of range": fts.Rank(tsv, q, 64).Gt(0),
		"two norms":         fts.Rank(tsv, q, 1, 2).Gt(0),
	}
	for name, p := range bad {
		if _, err := exprtest.Compile(dialect.Postgres, p); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s = %v, want ErrValidation", name, err)
		}
	}
}
