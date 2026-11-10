// SPDX-License-Identifier: Apache-2.0

//go:build integration

package jsonop_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/arrayop"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/jsonop"
	"github.com/azrtydxb/momo/types"
)

// Catches: a JSON or array operator whose SQL or bound arguments differ from
// the documented ones on PostgreSQL or SQLite, one that matches the wrong rows
// in the database, a value interpolated into the SQL text instead of bound,
// and an operator that fails silently instead of ErrUnsupported where the
// dialect has no equivalent.
func TestJSONAndArrayOperators(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		sqlite := d == dialect.SQLite
		ctx := context.Background()
		docType, tagsDDL := "jsonb", ", tags text[] NOT NULL DEFAULT '{}'"
		if sqlite {
			docType, tagsDDL = "text", ""
		}
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS jsonarr",
			"CREATE TABLE jsonarr (id bigint PRIMARY KEY, doc " + docType + tagsDDL + ")",
		} {
			if _, err := momo.Exec(ctx, db, stmt); err != nil {
				t.Fatal(err)
			}
		}
		seed := []struct {
			id   int64
			doc  string
			tags []string
		}{
			{1, `{"via":"api"}`, []string{"a", "b"}},
			{2, `{"via":"web"}`, []string{"c"}},
			{3, `{"other":1}`, []string{"a", "b", "c"}},
		}
		for _, r := range seed {
			var err error
			if sqlite {
				_, err = momo.Exec(ctx, db, "INSERT INTO jsonarr (id, doc) VALUES (?, ?)", r.id, r.doc)
			} else {
				_, err = momo.Exec(ctx, db, "INSERT INTO jsonarr (id, doc, tags) VALUES (?, CAST(? AS jsonb), ?)", r.id, r.doc, r.tags)
			}
			if err != nil {
				t.Fatal(err)
			}
		}

		id := momo.ColumnOf[int64]("jsonarr", "id")
		doc := momo.NewColumn[types.JSON[any]]("jsonarr", "doc", 0)
		tags := momo.NewColumn[[]string]("jsonarr", "tags", 0)
		query := func(p momo.Predicate) momo.SelectQuery[exprtest.Pair] {
			return momo.Select[exprtest.Pair](db).From(momo.TableRef{Name: "jsonarr"}).Columns(id, id).Where(p).OrderBy(id.Asc())
		}

		const hostile = "x' OR '1'='1"
		cases := []struct {
			name    string
			pred    momo.Predicate
			pgSQL   string // the WHERE text on PostgreSQL
			pgArgs  []any
			litSQL  string // the WHERE text on SQLite; empty means ErrUnsupported
			litArgs []any
			rows    []int64 // the matching ids where the dialect runs the operator
		}{
			{"JSON text equals", jsonop.Text(doc, "via").Eq("api"),
				`("jsonarr"."doc" ->> 'via') = $1`, []any{"api"},
				`CAST(json_extract("jsonarr"."doc", '$."via"') AS TEXT) = ?`, []any{"api"}, []int64{1}},
			{"JSON text equals a hostile value", jsonop.Text(doc, "via").Eq(hostile),
				`("jsonarr"."doc" ->> 'via') = $1`, []any{hostile},
				`CAST(json_extract("jsonarr"."doc", '$."via"') AS TEXT) = ?`, []any{hostile}, nil},
			{"JSON has key", jsonop.HasKey(doc, "via"),
				`("jsonarr"."doc" ? $1)`, []any{"via"},
				`(json_type("jsonarr"."doc", ?) IS NOT NULL)`, []any{`$."via"`}, []int64{1, 2}},
			{"JSON contains", jsonop.Contains(doc, map[string]any{"via": "web"}),
				`("jsonarr"."doc" @> CAST($1 AS jsonb))`, []any{`{"via":"web"}`},
				"", nil, []int64{2}},
			{"array contains", arrayop.Contains(tags, "a", "b"),
				`"jsonarr"."tags" @> $1`, []any{[]string{"a", "b"}}, "", nil, []int64{1, 3}},
			{"array contained by", arrayop.ContainedBy(tags, "a", "b", "c"),
				`"jsonarr"."tags" <@ $1`, []any{[]string{"a", "b", "c"}}, "", nil, []int64{1, 2, 3}},
			{"array overlaps", arrayop.Overlaps(tags, "b", "z"),
				`"jsonarr"."tags" && $1`, []any{[]string{"b", "z"}}, "", nil, []int64{1, 3}},
			{"column equals any of", arrayop.AnyOf(id, 1, 3),
				`"jsonarr"."id" = ANY($1)`, []any{[]int64{1, 3}}, "", nil, []int64{1, 3}},
		}
		for _, c := range cases {
			wantSQL, wantArgs := c.pgSQL, c.pgArgs
			if sqlite {
				wantSQL, wantArgs = c.litSQL, c.litArgs
			}
			q := query(c.pred)
			compiled, err := q.SQL()
			if wantSQL == "" {
				if !errors.Is(err, momo.ErrUnsupported) {
					t.Errorf("%s on %s: err = %v, want ErrUnsupported", c.name, d, err)
				}
				continue
			}
			if err != nil {
				t.Errorf("%s on %s: %v", c.name, d, err)
				continue
			}
			if !strings.HasSuffix(compiled.SQL, " WHERE "+wantSQL+` ORDER BY "jsonarr"."id" ASC`) {
				t.Errorf("%s on %s:\n got %s\nwant WHERE %s", c.name, d, compiled.SQL, wantSQL)
			}
			if !reflect.DeepEqual(compiled.Args, wantArgs) {
				t.Errorf("%s on %s: args = %#v, want %#v", c.name, d, compiled.Args, wantArgs)
			}
			if strings.Contains(compiled.SQL, hostile) || strings.Contains(compiled.SQL, "OR '1'") {
				t.Errorf("%s on %s: a value was interpolated into the SQL: %s", c.name, d, compiled.SQL)
			}
			got, err := q.All(ctx)
			if err != nil {
				t.Errorf("%s on %s: run: %v", c.name, d, err)
				continue
			}
			ids := []int64{}
			for _, r := range got {
				ids = append(ids, r.K)
			}
			if !slices.Equal(ids, append([]int64{}, c.rows...)) {
				t.Errorf("%s on %s: ids = %v, want %v", c.name, d, ids, c.rows)
			}
		}
	})
}
