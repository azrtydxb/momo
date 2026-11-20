// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests exercise the deprecated names

package fts_test

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/fts"
	"github.com/azrtydxb/momo/internal/exprtest"
)

// Catches: a deprecated query builder or to_tsvector name that stops
// forwarding to its TS-prefixed replacement, so a caller that has not migrated
// yet builds different SQL or binds different arguments.
func TestDeprecatedBuildersMatchReplacements(t *testing.T) {
	cases := []struct {
		name     string
		old, now momo.Expr
	}{
		{"ToVector", fts.ToVector("english", title), fts.TSVector("english", title)},
		{"WebsearchToQuery", fts.WebsearchToQuery("english", "red fox"), fts.TSWebsearchQuery("english", "red fox")},
		{"PlainToQuery", fts.PlainToQuery("english", "red fox"), fts.TSPlainQuery("english", "red fox")},
		{"PhraseToQuery", fts.PhraseToQuery("english", "red fox"), fts.TSPhraseQuery("english", "red fox")},
		{"ToQuery", fts.ToQuery("english", "red & fox:*"), fts.TSQuery("english", "red & fox:*")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertSameSQL(t, compile, c.old, c.now)
		})
	}
}

// Catches: a deprecated rank or headline name that drops the normalization
// argument or the options string when it forwards, or stops forwarding at all.
func TestDeprecatedRankAndHeadlineMatchReplacements(t *testing.T) {
	q := fts.TSWebsearchQuery("english", "red fox")
	cases := []struct {
		name     string
		old, now momo.Expr
	}{
		{"Rank", fts.Rank(tsv, q), fts.TSRank(tsv, q)},
		{"Rank with normalization", fts.Rank(tsv, q, 33), fts.TSRank(tsv, q, 33)},
		{"RankCD", fts.RankCD(tsv, q), fts.TSRankCD(tsv, q)},
		{"RankCD with normalization", fts.RankCD(tsv, q, 2), fts.TSRankCD(tsv, q, 2)},
		{"Headline", fts.Headline("english", title, q, "StartSel=<b>"), fts.TSHeadline("english", title, q, "StartSel=<b>")},
		{"Headline without options", fts.Headline("english", title, q, ""), fts.TSHeadline("english", title, q, "")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertSameSQL(t, compile, c.old, c.now)
		})
	}
}

// Catches: the deprecated Match predicate no longer rendering as TSMatch, or
// no longer failing the build on a dialect without PostgreSQL full-text search.
func TestDeprecatedMatchMatchesReplacement(t *testing.T) {
	q := fts.TSWebsearchQuery("english", "red fox")
	old, now := fts.Match(tsv, q), fts.TSMatch(tsv, q)

	got, err := compilePred(dialect.Postgres, old)
	if err != nil {
		t.Fatal(err)
	}
	want, err := compilePred(dialect.Postgres, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.SQL != want.SQL || !reflect.DeepEqual(got.Args, want.Args) {
		t.Errorf("Match = %s %#v\nTSMatch = %s %#v", got.SQL, got.Args, want.SQL, want.Args)
	}

	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		_, errOld := compilePred(d, old)
		_, errNow := compilePred(d, now)
		if (errOld == nil) != (errNow == nil) || (errOld != nil && errOld.Error() != errNow.Error()) {
			t.Errorf("%s: Match error %v, TSMatch error %v", d, errOld, errNow)
		}
	}
}

// compilePred renders p as the WHERE of a count over the articles table, the
// table the fixture columns belong to.
func compilePred(d dialect.Name, p momo.Predicate) (momo.Compiled, error) {
	return momo.Select[exprtest.Tally](exprtest.DB(d)).
		From(momo.TableRef{Name: "articles"}).
		Columns(momo.CountAll()).
		Where(p).
		SQL()
}

// assertSameSQL renders old and now through build and fails unless the SQL text
// and the bound arguments are identical.
func assertSameSQL(t *testing.T, build func(momo.Expr, func(tallyQuery) tallyQuery) (momo.Compiled, error), old, now momo.Expr) {
	t.Helper()
	got, err := build(old, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, err := build(now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.SQL != want.SQL {
		t.Errorf("SQL\n got %s\nwant %s", got.SQL, want.SQL)
	}
	if !reflect.DeepEqual(got.Args, want.Args) {
		t.Errorf("args = %#v, want %#v", got.Args, want.Args)
	}
}
