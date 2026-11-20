// SPDX-License-Identifier: Apache-2.0

package fts_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/fts"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/schema"
)

type article struct {
	ID          int64
	Title, Body string
}

var articleMeta = momo.Meta[article]{
	Table:    "articles",
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "title"}, {Name: "body"}},
	Ptr:      func(m *article, i int) any { return []any{&m.ID, &m.Title, &m.Body}[i] },
	Value:    func(m *article, i int) any { return []any{m.ID, m.Title, m.Body}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
	TableSchema: &schema.Table{
		Name:       "articles",
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{
			Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"}},
		}},
	},
}

func (a *article) MomoMeta() *momo.Meta[article] { return &articleMeta }

var (
	artTitle = momo.NewColumn[string]("articles", "title", 0)
	artBody  = momo.NewColumn[string]("articles", "body", 0)
)

// oneValue projects a single select-list expression.
type oneValue struct{ V any }

var oneValueMeta = momo.Meta[oneValue]{
	Columns:  []momo.ColumnMeta{{Name: "v"}},
	Ptr:      func(m *oneValue, i int) any { return &m.V },
	Value:    func(m *oneValue, i int) any { return m.V },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*oneValue) MomoMeta() *momo.Meta[oneValue] { return &oneValueMeta }

func compileFor(t *testing.T, d dialect.Name, e momo.Expr, p momo.Predicate) (momo.Compiled, error) {
	t.Helper()
	db := momo.New(fakedriver.New(d))
	q := momo.Select[oneValue](db).From(momo.TableRef{Name: "articles"}).Columns(e)
	if p != nil {
		q = q.Where(p)
	}
	return q.SQL()
}

// Catches: user text in the SQL string, a changed placeholder order, the
// configuration not pinned as a literal, bm25 not negated, and a changed
// SQL shape on either dialect.
func TestFullTextSearchGolden(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	q := fts.Web(`red fox`)
	cases := []struct {
		name string
		d    dialect.Name
		e    momo.Expr
		p    momo.Predicate
		want string
		args []any
	}{
		{"pg search", dialect.Postgres, momo.CountAll(), fts.Search(target, q),
			`SELECT COUNT(*) FROM "articles" WHERE "articles"."articles_search_tsv" @@ websearch_to_tsquery(CAST('english' AS regconfig), $1)`,
			[]any{"red fox"}},
		{"pg score", dialect.Postgres, fts.Score(target, q).As("score"), nil,
			`SELECT ts_rank_cd("articles"."articles_search_tsv", websearch_to_tsquery(CAST('english' AS regconfig), $1)) AS "score" FROM "articles"`,
			[]any{"red fox"}},
		{"pg prefix", dialect.Postgres, momo.CountAll(), fts.Search(target, fts.Web(`red fo*`)),
			`WHERE "articles"."articles_search_tsv" @@ to_tsquery(CAST('english' AS regconfig), $1)`,
			[]any{`'red' & 'fo':*`}},
		{"sqlite search", dialect.SQLite, momo.CountAll(), fts.Search(target, q),
			`WHERE "articles"."id" IN (SELECT rowid FROM "articles_fts" WHERE "articles_fts" MATCH ?)`,
			[]any{`"red" "fox"`}},
		{"sqlite score", dialect.SQLite, fts.Score(target, q).As("score"), nil,
			`SELECT COALESCE((SELECT -bm25("articles_fts", 1, 0.4) FROM "articles_fts" WHERE "articles_fts" MATCH ? AND "articles_fts".rowid = "articles"."id"), 0) AS "score" FROM "articles"`,
			[]any{`"red" "fox"`}},
	}
	for _, c := range cases {
		got, err := compileFor(t, c.d, c.e, c.p)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !strings.Contains(got.SQL, c.want) {
			t.Errorf("%s: SQL\n got %s\nwant it to contain %s", c.name, got.SQL, c.want)
		}
		if !reflect.DeepEqual(got.Args, c.args) {
			t.Errorf("%s: args %#v, want %#v", c.name, got.Args, c.args)
		}
	}
}

// Catches: user text interpolated into the SQL text on any dialect.
func TestFullTextQueryIsBound(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	const hostile = `x') OR 1=1 --`
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		got, err := compileFor(t, d, momo.CountAll(), fts.Search(target, fts.Web(hostile)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got.SQL, "OR 1=1") || strings.Contains(got.SQL, "x'") {
			t.Errorf("%s: user text reached the SQL: %s", d, got.SQL)
		}
	}
}

// Catches: a MATCH sent for an empty query, and a row returned for it.
func TestFullTextEmptyQuery(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		for _, text := range []string{"", "   ", "-spam", "NEAR( ("} {
			got, err := compileFor(t, d, momo.CountAll(), fts.Search(target, fts.Web(text)))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got.SQL, "MATCH") || strings.Contains(got.SQL, "@@") || !strings.Contains(got.SQL, "1 = 0") || len(got.Args) != 0 {
				t.Errorf("%s %q: %s %v", d, text, got.SQL, got.Args)
			}
			sc, err := compileFor(t, d, fts.Score(target, fts.Web(text)).As("s"), nil)
			if err != nil || !strings.Contains(sc.SQL, "SELECT 0 AS") {
				t.Errorf("%s %q: score = %v %v", d, text, sc.SQL, err)
			}
		}
	}
}

// Catches: a dialect-specific predicate building SQL on the wrong dialect.
func TestFullTextWrongDialectUnsupported(t *testing.T) {
	cases := []struct {
		d dialect.Name
		p momo.Predicate
	}{
		{dialect.Postgres, fts.FTS5Match("articles_fts", `"x"`)},
		{dialect.MySQL, fts.FTS5Match("articles_fts", `"x"`)},
		{dialect.SQLite, fts.TSMatch(tsv, fts.TSWebsearchQuery("english", "x"))},
		{dialect.MySQL, fts.TSMatch(tsv, fts.TSWebsearchQuery("english", "x"))},
	}
	for _, c := range cases {
		db := momo.New(fakedriver.New(c.d))
		_, err := momo.Select[article](db).From(momo.TableRef{Name: "articles"}).Where(c.p).SQL()
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", c.d, err)
		}
	}
}

// Catches: a config override that differs from the index config accepted, and
// a target that is not the declared column set accepted.
func TestFullTextTargetValidation(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	_, err := compileFor(t, dialect.Postgres, momo.CountAll(), fts.Search(target, fts.Web("x").Config("simple")))
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("config override: err = %v, want ErrValidation", err)
	}
	_, err = compileFor(t, dialect.SQLite, momo.CountAll(), fts.Search(fts.Over[article](artTitle), fts.Web("x")))
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("undeclared column set: err = %v, want ErrValidation", err)
	}
	_, err = compileFor(t, dialect.MySQL, momo.CountAll(), fts.Search(fts.Over[article](artBody, artTitle), fts.Web("x")))
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("reordered column set on MySQL: err = %v, want ErrValidation", err)
	}
}

// Catches: a MySQL search that fails on a config override it must ignore (the
// FULLTEXT index has no configuration).
func TestFullTextMySQLIgnoresConfig(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	if _, err := compileFor(t, dialect.MySQL, momo.CountAll(), fts.Search(target, fts.Web("x").Config("simple"))); err != nil {
		t.Errorf("config override on MySQL: %v", err)
	}
}

// Catches: the portable calls binding the query text on the plain argument
// path, so a hook sees text searched in a sensitive column, and masking the
// arguments of a plain column.
func TestFullTextHooksSeeMaskedArgsPortable(t *testing.T) {
	secretTitle := momo.NewColumn[string]("articles", "title", momo.FlagSensitive)
	cases := []struct {
		name   string
		target fts.Target
		masked bool
	}{
		{"sensitive title", fts.Over[article](secretTitle, artBody), true},
		{"plain columns", fts.Over[article](artTitle, artBody), false},
	}
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		for _, c := range cases {
			t.Run(string(d)+" "+c.name, func(t *testing.T) {
				q := fts.Web("hunter2 sensitive")
				rec := &obstest.Recorder{}
				drv := fakedriver.New(d)
				db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
				_, err := momo.Select[article](db).
					Where(fts.Search(c.target, q)).
					OrderBy(fts.Score(c.target, q).Desc()).
					All(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				calls, evs := drv.Calls(), rec.Events()
				if len(calls) == 0 || len(evs) == 0 {
					t.Fatalf("calls %d, events %d", len(calls), len(evs))
				}
				sent, seen := calls[len(calls)-1].Args, evs[len(evs)-1].Args
				if len(sent) == 0 || len(sent) != len(seen) {
					t.Fatalf("driver got %d args, hook saw %d", len(sent), len(seen))
				}
				for i := range sent {
					if sent[i] == momo.RedactedArg {
						t.Errorf("arg %d: the driver got the placeholder", i)
					}
					if (seen[i] == momo.RedactedArg) != c.masked {
						t.Errorf("arg %d: hook saw %v, masked want %v", i, seen[i], c.masked)
					}
				}
			})
		}
	}
}

// Catches: an out-of-range snippet token count accepted, a snippet column that
// is not in the declaration accepted, and the SQLite snippet shape.
func TestFullTextSnippetBuild(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	q := fts.Web("fox")
	got, err := compileFor(t, dialect.SQLite, fts.Snippet(target, q, fts.SnippetOpts{Column: artBody}).As("s"), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT (SELECT snippet("articles_fts", 1, ?, ?, ?, 16) FROM "articles_fts" WHERE "articles_fts" MATCH ? AND "articles_fts".rowid = "articles"."id") AS "s" FROM "articles"`
	if got.SQL != want || !reflect.DeepEqual(got.Args, []any{"<b>", "</b>", "...", `"fox"`}) {
		t.Errorf("sqlite snippet:\n got %s %#v\nwant %s", got.SQL, got.Args, want)
	}
	pg, err := compileFor(t, dialect.Postgres, fts.Snippet(target, q, fts.SnippetOpts{Column: artTitle}).As("s"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pg.SQL, `ts_headline(CAST('english' AS regconfig), "articles"."title", websearch_to_tsquery(CAST('english' AS regconfig), $1), $2)`) ||
		!reflect.DeepEqual(pg.Args, []any{"fox", "StartSel=<b>, StopSel=</b>"}) {
		t.Errorf("pg snippet: %s %#v", pg.SQL, pg.Args)
	}
	other := momo.NewColumn[string]("articles", "slug", 0)
	for _, o := range []fts.SnippetOpts{{Column: other}, {Column: artBody, Tokens: 65}, {Column: artBody, Tokens: -1}} {
		if _, err := compileFor(t, dialect.SQLite, fts.Snippet(target, q, o).As("s"), nil); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("snippet %+v: err = %v, want ErrValidation", o, err)
		}
	}
}

// Catches: a snippet without a column to excerpt reaching the database on a
// hand-managed tsvector target, where no declaration lists the columns.
func TestFullTextSnippetNeedsColumn(t *testing.T) {
	target := fts.OverVector(tsv, "english")
	_, err := compileFor(t, dialect.Postgres, fts.Snippet(target, fts.Web("fox"), fts.SnippetOpts{}).As("s"), nil)
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}
	got, err := compileFor(t, dialect.Postgres, fts.Snippet(target, fts.Web("fox"), fts.SnippetOpts{Column: artBody}).As("s"), nil)
	if err != nil || !strings.Contains(got.SQL, `ts_headline(CAST($1 AS regconfig), "articles"."body"`) {
		t.Errorf("snippet over a tsvector column: %v %v", got.SQL, err)
	}
}

// Catches: the MySQL boolean-mode shape changing, the query text leaving the
// bound argument, a score that is not the MATCH value, and a snippet building
// on a server without one.
func TestFullTextMySQLGolden(t *testing.T) {
	target := fts.Over[article](artTitle, artBody)
	got, err := compileFor(t, dialect.MySQL, momo.CountAll(), fts.Search(target, fts.Web(`red fox -hen`)))
	if err != nil {
		t.Fatal(err)
	}
	want := "WHERE MATCH(`articles`.`title`, `articles`.`body`) AGAINST (? IN BOOLEAN MODE)"
	if !strings.Contains(got.SQL, want) || !reflect.DeepEqual(got.Args, []any{`+red +fox -hen`}) {
		t.Fatalf("SQL %s args %v", got.SQL, got.Args)
	}
	sc, err := compileFor(t, dialect.MySQL, fts.Score(target, fts.Web(`red fox`)).As("score"), nil)
	if err != nil {
		t.Fatal(err)
	}
	wantScore := "SELECT MATCH(`articles`.`title`, `articles`.`body`) AGAINST (? IN BOOLEAN MODE) AS `score` FROM `articles`"
	if sc.SQL != wantScore || !reflect.DeepEqual(sc.Args, []any{`+red +fox`}) {
		t.Fatalf("score SQL %s args %v", sc.SQL, sc.Args)
	}
	raw, err := compileFor(t, dialect.MySQL, momo.CountAll(), fts.Search(target, fts.Raw(`+go -orm`)))
	if err != nil || !reflect.DeepEqual(raw.Args, []any{`+go -orm`}) {
		t.Fatalf("raw: %v %v", err, raw.Args)
	}
	sn := fts.Snippet(target, fts.Web("x"), fts.SnippetOpts{Column: artTitle})
	if _, err := compileFor(t, dialect.MySQL, sn.As("s"), nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("snippet on MySQL: err = %v, want ErrUnsupported", err)
	}
	if _, err := compileFor(t, dialect.MySQL, momo.CountAll(), fts.Search(fts.OverVector(tsv, "english"), fts.Web("x"))); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("tsvector target on MySQL: err = %v, want ErrUnsupported", err)
	}
}

// Catches: the mode text or the bound query changing, and the mode taken from
// input.
func TestFullTextMySQLMatchGolden(t *testing.T) {
	cols := []momo.Column[string]{artTitle, artBody}
	for mode, want := range map[fts.MySQLMode]string{
		fts.NaturalLanguage:              "MATCH(`articles`.`title`, `articles`.`body`) AGAINST (? IN NATURAL LANGUAGE MODE)",
		fts.Boolean:                      "MATCH(`articles`.`title`, `articles`.`body`) AGAINST (? IN BOOLEAN MODE)",
		fts.NaturalLanguageWithExpansion: "MATCH(`articles`.`title`, `articles`.`body`) AGAINST (? IN NATURAL LANGUAGE MODE WITH QUERY EXPANSION)",
	} {
		got, err := compileFor(t, dialect.MySQL, momo.CountAll(), fts.MySQLMatch(cols, "+go -orm", mode))
		if err != nil || !strings.Contains(got.SQL, want) || !reflect.DeepEqual(got.Args, []any{"+go -orm"}) {
			t.Errorf("mode %d: %v\n%s\n%v", mode, err, got.SQL, got.Args)
		}
		rel, err := compileFor(t, dialect.MySQL, fts.MySQLRelevance(cols, "go", mode).As("r"), nil)
		if err != nil || !strings.Contains(rel.SQL, want) || !reflect.DeepEqual(rel.Args, []any{"go"}) {
			t.Errorf("relevance mode %d: %v\n%s\n%v", mode, err, rel.SQL, rel.Args)
		}
	}
	if _, err := compileFor(t, dialect.MySQL, momo.CountAll(), fts.MySQLMatch(cols, "x", fts.MySQLMode(9))); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("unknown mode: err = %v, want ErrValidation", err)
	}
	if _, err := compileFor(t, dialect.MySQL, momo.CountAll(), fts.MySQLMatch(nil, "x", fts.Boolean)); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("no columns: err = %v, want ErrValidation", err)
	}
	if _, err := compileFor(t, dialect.Postgres, momo.CountAll(), fts.MySQLMatch(cols, "x", fts.Boolean)); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("MySQLMatch on PostgreSQL: err = %v, want ErrUnsupported", err)
	}
}

// Catches: the FTS5 match and bm25 escape hatches rendering a different shape,
// and a non-finite bm25 weight reaching SQL.
func TestFullTextFTS5Builders(t *testing.T) {
	got, err := compileFor(t, dialect.SQLite, fts.BM25("articles_fts", 1, 0.5).As("r"), fts.FTS5Match("articles_fts", `"fox"`))
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT bm25("articles_fts", 1, 0.5) AS "r" FROM "articles" WHERE "articles_fts" MATCH ?`
	if got.SQL != want || !reflect.DeepEqual(got.Args, []any{`"fox"`}) {
		t.Errorf("got %s %#v", got.SQL, got.Args)
	}
}
