// SPDX-License-Identifier: Apache-2.0

//go:build integration

package fts_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/fts"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// The migrations that derive the structures from a declaration land in other
// changes, so this test creates them with the SQL those migrations render: a
// stored generated tsvector column with a GIN index on PostgreSQL, an
// external-content FTS5 table on SQLite.

var artID = momo.NewColumn[int64]("articles", "id", 0)

// artIDRow projects the id of an article.
type artIDRow struct{ ID int64 }

var artIDRowMeta = momo.Meta[artIDRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}},
	Ptr:      func(m *artIDRow, i int) any { return &m.ID },
	Value:    func(m *artIDRow, i int) any { return m.ID },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*artIDRow) MomoMeta() *momo.Meta[artIDRow] { return &artIDRowMeta }

// artScoreRow projects an id and a score.
type artScoreRow struct {
	ID    int64
	Score float64
}

var artScoreRowMeta = momo.Meta[artScoreRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "score"}},
	Ptr:      func(m *artScoreRow, i int) any { return [...]any{&m.ID, &m.Score}[i] },
	Value:    func(m *artScoreRow, i int) any { return [...]any{m.ID, m.Score}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*artScoreRow) MomoMeta() *momo.Meta[artScoreRow] { return &artScoreRowMeta }

// artSnipRow projects an id and a text.
type artSnipRow struct {
	ID   int64
	Text string
}

var artSnipRowMeta = momo.Meta[artSnipRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "text"}},
	Ptr:      func(m *artSnipRow, i int) any { return [...]any{&m.ID, &m.Text}[i] },
	Value:    func(m *artSnipRow, i int) any { return [...]any{m.ID, m.Text}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*artSnipRow) MomoMeta() *momo.Meta[artSnipRow] { return &artSnipRowMeta }

func execAll(t *testing.T, db *momo.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.Driver().Exec(context.Background(), s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// createArticles creates the articles table and the search structures of the
// declaration in articleMeta, then inserts rows (id, title, body; "" is NULL).
func createArticles(t *testing.T, db *momo.DB, d dialect.Name, rows [][3]string) {
	t.Helper()
	switch d {
	case dialect.Postgres:
		execAll(t, db,
			`CREATE TABLE articles (id bigint PRIMARY KEY, title text, body text)`,
			`ALTER TABLE articles ADD COLUMN articles_search_tsv tsvector GENERATED ALWAYS AS (
				setweight(to_tsvector('english', coalesce("title", '')), 'A') ||
				setweight(to_tsvector('english', coalesce("body", '')), 'B')) STORED`,
			`CREATE INDEX articles_search ON articles USING gin (articles_search_tsv)`)
	case dialect.SQLite:
		execAll(t, db,
			`CREATE TABLE articles (id INTEGER PRIMARY KEY, title TEXT, body TEXT)`,
			`CREATE VIRTUAL TABLE articles_fts USING fts5(title, body, content='articles', content_rowid='id')`)
	default:
		t.Fatalf("dialect %s is not covered here", d)
	}
	lit := func(s string) string {
		if s == "" {
			return "NULL"
		}
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	for _, r := range rows {
		execAll(t, db, fmt.Sprintf(`INSERT INTO articles (id, title, body) VALUES (%s, %s, %s)`, r[0], lit(r[1]), lit(r[2])))
	}
	if d == dialect.SQLite {
		execAll(t, db, `INSERT INTO articles_fts(articles_fts) VALUES('rebuild')`)
	}
}

// seedArticles inserts the six rows the expectations below are written for.
func seedArticles(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Helper()
	createArticles(t, db, d, [][3]string{
		{"1", "red fox", "the red fox jumps"},
		{"2", "blue whale", "slow and large"},
		{"3", "red hen", "a red hen and a fox"},
		{"4", "fox red", "fast animal"},
		{"5", "foxtrot", "a dance"},
		{"6", "green frog", ""},
	})
}

func searchIDs(t *testing.T, db *momo.DB, q fts.Query) []int64 {
	t.Helper()
	target := fts.Over[article](artTitle, artBody)
	out, err := momo.Select[artIDRow](db).From(momo.TableRef{Name: "articles"}).Columns(artID).
		Where(fts.Search(target, q)).OrderBy(artID.Asc()).All(context.Background())
	if err != nil {
		t.Fatalf("search %+v: %v", q, err)
	}
	ids := []int64{}
	for _, r := range out {
		ids = append(ids, r.ID)
	}
	return ids
}

// Catches: a dialect returning a row that misses a word, or omitting a row that
// has all of them (S-1), and the portable score ordering being inverted (S-2).
func TestFullTextSearchAllDialects(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedArticles(t, db, d)
		if got := searchIDs(t, db, fts.Web(`red fox`)); !reflect.DeepEqual(got, []int64{1, 3, 4}) {
			t.Fatalf("all words: %v, want [1 3 4]", got)
		}
		if got := searchIDs(t, db, fts.Web(`"red fox" -hen`)); !reflect.DeepEqual(got, []int64{1}) {
			t.Fatalf("phrase and exclusion: %v, want [1]", got)
		}
		if got := searchIDs(t, db, fts.Web(`fo*`)); !reflect.DeepEqual(got, []int64{1, 3, 4, 5}) {
			t.Fatalf("prefix: %v, want [1 3 4 5]", got)
		}
		if got := searchIDs(t, db, fts.Plain(`red fox`)); !reflect.DeepEqual(got, []int64{1, 3, 4}) {
			t.Fatalf("plain: %v, want [1 3 4]", got)
		}
	})
}

// Catches: a score that does not order the row with the most occurrences first
// (bm25 not negated, or the order inverted).
func TestFullTextScoreOrdering(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createArticles(t, db, d, [][3]string{
			{"1", "note", "fox cat dog"},
			{"2", "note", "fox fox dog"},
			{"3", "note", "fox fox fox"},
			{"4", "note", "cat dog bird"},
			{"5", "note", "bird cat cat"},
			{"6", "note", "dog dog bird"},
		})
		target := fts.Over[article](artTitle, artBody)
		q := fts.Web("fox")
		out, err := momo.Select[artIDRow](db).From(momo.TableRef{Name: "articles"}).Columns(artID).
			Where(fts.Search(target, q)).OrderBy(fts.Score(target, q).Desc()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, r := range out {
			got = append(got, r.ID)
		}
		if !reflect.DeepEqual(got, []int64{3, 2, 1}) {
			t.Fatalf("score order %v, want [3 2 1]", got)
		}
	})
}

// Catches: a row the query does not match scoring NULL on SQLite, which sorts
// below every score on one dialect and above it on another, instead of 0.
func TestFullTextScoreWithoutSearch(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createArticles(t, db, d, [][3]string{
			{"1", "note", "fox cat dog"},
			{"2", "note", "fox fox dog"},
			{"3", "note", "fox fox fox"},
			{"4", "note", "cat dog bird"},
			{"5", "note", "bird cat cat"},
			{"6", "note", "dog dog bird"},
		})
		target := fts.Over[article](artTitle, artBody)
		q := fts.Web("fox")
		out, err := momo.Select[artIDRow](db).From(momo.TableRef{Name: "articles"}).Columns(artID).
			OrderBy(fts.Score(target, q).Desc(), artID.Asc()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, r := range out {
			got = append(got, r.ID)
		}
		if !reflect.DeepEqual(got, []int64{3, 2, 1, 4, 5, 6}) {
			t.Fatalf("order by score without a search: %v, want [3 2 1 4 5 6]", got)
		}
		sc, err := momo.Select[artScoreRow](db).From(momo.TableRef{Name: "articles"}).
			Columns(artID, fts.Score(target, q).As("score")).Where(artID.Eq(4)).All(context.Background())
		if err != nil || len(sc) != 1 || sc[0].Score != 0 {
			t.Fatalf("score of a non-matching row = %+v, %v; want 0", sc, err)
		}
	})
}

// Catches: a database syntax error for user text on any dialect.
func TestFullTextWebQueryNeverErrors(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedArticles(t, db, d)
		inputs := []string{`a OR b -c "d e" NEAR( (`, `"`, `((`, `-`, `*`, `\`, `'`, ``, `  `, `OR`, `-"`, `"*`, `a*b`}
		rng := rand.New(rand.NewSource(7))
		const alphabet = "ab c\"'()-*:^\\OR&|!<>~+@%_.,é漢"
		runes := []rune(alphabet)
		for i := 0; i < 200; i++ {
			n := rng.Intn(14)
			var sb strings.Builder
			for j := 0; j < n; j++ {
				sb.WriteRune(runes[rng.Intn(len(runes))])
			}
			inputs = append(inputs, sb.String())
		}
		for _, in := range inputs {
			for _, q := range []fts.Query{fts.Web(in), fts.Plain(in)} {
				searchIDs(t, db, q)
			}
		}
	})
}

// Catches: a phrase matching non-adjacent words, and a prefix missing a row.
func TestFullTextPhraseAndPrefix(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedArticles(t, db, d)
		if got := searchIDs(t, db, fts.Web(`"red fox"`)); !reflect.DeepEqual(got, []int64{1}) {
			t.Fatalf("phrase: %v, want [1]", got)
		}
		if got := searchIDs(t, db, fts.Web(`"fox red"`)); !reflect.DeepEqual(got, []int64{4}) {
			t.Fatalf("reversed phrase: %v, want [4]", got)
		}
		if got := searchIDs(t, db, fts.Web(`fo*`)); !reflect.DeepEqual(got, []int64{1, 3, 4, 5}) {
			t.Fatalf("prefix: %v, want [1 3 4 5]", got)
		}
	})
}

// Catches: an excluded row returned, and a negation-only query erroring or
// matching rows.
func TestFullTextExclusion(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedArticles(t, db, d)
		if got := searchIDs(t, db, fts.Web(`fox -hen`)); !reflect.DeepEqual(got, []int64{1, 4}) {
			t.Fatalf("exclusion: %v, want [1 4]", got)
		}
		if got := searchIDs(t, db, fts.Web(`-fox`)); len(got) != 0 {
			t.Fatalf("negation only: %v, want no rows", got)
		}
	})
}

// Catches: a snippet without the match marks.
func TestFullTextSnippet(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedArticles(t, db, d)
		target := fts.Over[article](artTitle, artBody)
		q := fts.Web("jumps")
		out, err := momo.Select[artSnipRow](db).From(momo.TableRef{Name: "articles"}).
			Columns(artID, fts.Snippet(target, q, fts.SnippetOpts{Column: artBody}).As("text")).
			Where(fts.Search(target, q)).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || out[0].ID != 1 || !strings.Contains(out[0].Text, "<b>jumps</b>") {
			t.Fatalf("snippet rows = %+v, want row 1 with <b>jumps</b>", out)
		}
	})
}

// Catches: a NULL source column hiding a row that has text in the other, and a
// row with every searchable column NULL being returned.
func TestFullTextNulls(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createArticles(t, db, d, [][3]string{
			{"1", "green frog", ""},
			{"2", "", "a frog sits"},
			{"3", "", ""},
			{"4", "plain", "text"},
		})
		if got := searchIDs(t, db, fts.Web("frog")); !reflect.DeepEqual(got, []int64{1, 2}) {
			t.Fatalf("frog: %v, want [1 2]", got)
		}
		if got := searchIDs(t, db, fts.Web("plain OR frog OR text")); !reflect.DeepEqual(got, []int64{1, 2, 4}) {
			t.Fatalf("any word: %v, want [1 2 4]", got)
		}
	})
}

// Catches: a missing FTS5 table surfacing as a raw driver string instead of
// ErrUnsupported naming the table.
func TestFullTextSQLiteMissingTable(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.SQLite {
			t.Skip("FTS5 tables exist on SQLite only")
		}
		execAll(t, db, `CREATE TABLE articles (id INTEGER PRIMARY KEY, title TEXT, body TEXT)`)
		target := fts.Over[article](artTitle, artBody)
		_, err := momo.Select[artIDRow](db).From(momo.TableRef{Name: "articles"}).Columns(artID).
			Where(fts.Search(target, fts.Web("x"))).All(context.Background())
		if !errors.Is(err, momo.ErrUnsupported) || err == nil || !strings.Contains(err.Error(), "articles_fts") {
			t.Fatalf("err = %v, want ErrUnsupported naming articles_fts", err)
		}
	})
}
