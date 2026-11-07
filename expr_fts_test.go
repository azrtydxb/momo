// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/types"
)

var (
	ftsTsv   = momo.NewColumn[types.TSVector]("articles", "search_tsv", 0)
	ftsTitle = momo.NewColumn[string]("articles", "title", 0)
)

// ftsRow is a three-field projection for the full-text goldens.
type ftsRow struct {
	A, B string
	N    float64
}

var ftsRowMeta = momo.Meta[ftsRow]{
	Columns:  []momo.ColumnMeta{{Name: "a"}, {Name: "b"}, {Name: "n"}},
	Ptr:      func(m *ftsRow, i int) any { return [...]any{&m.A, &m.B, &m.N}[i] },
	Value:    func(m *ftsRow, i int) any { return [...]any{m.A, m.B, m.N}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*ftsRow) MomoMeta() *momo.Meta[ftsRow] { return &ftsRowMeta }

// TestFullTextGolden pins the SQL and arguments of every full-text builder in
// Where, OrderBy and Columns on PostgreSQL.
func TestFullTextGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	q := momo.WebsearchToTSQuery("english", "red fox")

	cases := []struct {
		name string
		q    func() (momo.Compiled, error)
		want string
		args []any
	}{
		{"match with websearch", func() (momo.Compiled, error) {
			return momo.Select[ftsRow](pg).From(momo.TableRef{Name: "articles"}).
				Columns(ftsTitle, ftsTitle, momo.CountAll()).
				Where(momo.TSMatch(ftsTsv, q)).SQL()
		}, `SELECT "articles"."title", "articles"."title", COUNT(*) FROM "articles" WHERE "articles"."search_tsv" @@ websearch_to_tsquery(CAST($1 AS regconfig), $2)`,
			[]any{"english", "red fox"}},
		{"plain phrase and to_tsquery", func() (momo.Compiled, error) {
			return momo.Select[ftsRow](pg).From(momo.TableRef{Name: "articles"}).
				Columns(ftsTitle, ftsTitle, momo.CountAll()).
				Where(momo.Or(
					momo.TSMatch(ftsTsv, momo.PlainToTSQuery("simple", "a b")),
					momo.TSMatch(ftsTsv, momo.PhraseToTSQuery("simple", "a b")),
					momo.TSMatch(ftsTsv, momo.ToTSQuery("simple", "a & b:*")),
				)).SQL()
		}, `WHERE ("articles"."search_tsv" @@ plainto_tsquery(CAST($1 AS regconfig), $2) OR "articles"."search_tsv" @@ phraseto_tsquery(CAST($3 AS regconfig), $4) OR "articles"."search_tsv" @@ to_tsquery(CAST($5 AS regconfig), $6))`,
			[]any{"simple", "a b", "simple", "a b", "simple", "a & b:*"}},
		{"to_tsvector on a text column", func() (momo.Compiled, error) {
			return momo.Select[ftsRow](pg).From(momo.TableRef{Name: "articles"}).
				Columns(ftsTitle, ftsTitle, momo.CountAll()).
				Where(momo.TSMatch(momo.ToTSVector("english", ftsTitle), q)).SQL()
		}, `WHERE to_tsvector(CAST($1 AS regconfig), "articles"."title") @@ websearch_to_tsquery(CAST($2 AS regconfig), $3)`,
			[]any{"english", "english", "red fox"}},
		{"rank in order by, descending", func() (momo.Compiled, error) {
			return momo.Select[ftsRow](pg).From(momo.TableRef{Name: "articles"}).
				Columns(ftsTitle, ftsTitle, momo.CountAll()).
				OrderBy(momo.TSRankCD(ftsTsv, q).Desc()).SQL()
		}, `ORDER BY ts_rank_cd("articles"."search_tsv", websearch_to_tsquery(CAST($1 AS regconfig), $2)) DESC`,
			[]any{"english", "red fox"}},
		{"rank with normalization in the select list", func() (momo.Compiled, error) {
			return momo.Select[ftsRow](pg).From(momo.TableRef{Name: "articles"}).
				Columns(ftsTitle, ftsTitle, momo.TSRank(ftsTsv, q, 33).As("rank")).SQL()
		}, `SELECT "articles"."title", "articles"."title", ts_rank("articles"."search_tsv", websearch_to_tsquery(CAST($1 AS regconfig), $2), 33) AS "rank" FROM`,
			[]any{"english", "red fox"}},
		{"headline with options", func() (momo.Compiled, error) {
			return momo.Select[ftsRow](pg).From(momo.TableRef{Name: "articles"}).
				Columns(ftsTitle, momo.TSHeadline("english", ftsTitle, q, "StartSel=<b>, StopSel=</b>"), momo.CountAll()).SQL()
		}, `ts_headline(CAST($1 AS regconfig), "articles"."title", websearch_to_tsquery(CAST($2 AS regconfig), $3), $4)`,
			[]any{"english", "english", "red fox", "StartSel=<b>, StopSel=</b>"}},
		{"headline without options", func() (momo.Compiled, error) {
			return momo.Select[ftsRow](pg).From(momo.TableRef{Name: "articles"}).
				Columns(ftsTitle, momo.TSHeadline("english", ftsTitle, q, ""), momo.CountAll()).SQL()
		}, `ts_headline(CAST($1 AS regconfig), "articles"."title", websearch_to_tsquery(CAST($2 AS regconfig), $3)), COUNT(*)`,
			[]any{"english", "english", "red fox"}},
	}
	for _, c := range cases {
		got, err := c.q()
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

// TestFullTextUnsupportedAndInvalid requires ErrUnsupported on the other
// dialects and ErrValidation for an empty configuration or a bad
// normalization, never a malformed statement.
func TestFullTextUnsupportedAndInvalid(t *testing.T) {
	q := momo.WebsearchToTSQuery("english", "x")
	build := func(d dialect.Name, p momo.Predicate) error {
		_, err := momo.Select[ftsRow](momo.New(fakedriver.New(d))).From(momo.TableRef{Name: "articles"}).
			Columns(ftsTitle, ftsTitle, momo.CountAll()).Where(p).SQL()
		return err
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if err := build(d, momo.TSMatch(ftsTsv, q)); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("TSMatch on %s = %v, want ErrUnsupported", d, err)
		}
		rank := momo.TSRankCD(ftsTsv, q).Gt(0.1)
		if err := build(d, rank); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("TSRankCD on %s = %v, want ErrUnsupported", d, err)
		}
	}
	bad := map[string]momo.Predicate{
		"empty config":      momo.TSMatch(ftsTsv, momo.WebsearchToTSQuery("", "x")),
		"empty vec config":  momo.TSMatch(momo.ToTSVector("", ftsTitle), q),
		"negative norm":     momo.TSRankCD(ftsTsv, q, -1).Gt(0),
		"norm out of range": momo.TSRank(ftsTsv, q, 64).Gt(0),
		"two norms":         momo.TSRank(ftsTsv, q, 1, 2).Gt(0),
	}
	for name, p := range bad {
		if err := build(dialect.Postgres, p); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s = %v, want ErrValidation", name, err)
		}
	}
}
