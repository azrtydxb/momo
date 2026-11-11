// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/arrayop"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
)

func arrayWhere(d dialect.Name, p momo.Predicate) (momo.Compiled, error) {
	return momo.Select[exprtest.Tally](exprtest.DB(d)).From(momo.TableRef{Name: "t"}).
		Columns(momo.CountAll()).Where(p).SQL()
}

func arraySub(d dialect.Name) momo.SelectQuery[exprtest.Tally] {
	return momo.Select[exprtest.Tally](exprtest.DB(d)).From(momo.TableRef{Name: "u"}).
		Columns(momo.ColumnOf[int64]("u", "id")).Where(momo.ColumnOf[int64]("u", "n").Gt(2))
}

// Catches: ArrayOf and ArraySubquery rendering the wrong constructor, losing
// the element expressions' bound arguments or the subquery's parentheses, and
// a set-operation subquery not collected.
func TestArrayConstructorsSQL(t *testing.T) {
	col := momo.ColumnOf[string]("t", "a")
	sub := arraySub(dialect.Postgres)
	cases := []struct {
		name string
		p    momo.Predicate
		want string
		args []any
	}{
		{"array of column and value", arrayop.Contains(momo.ArrayOf[string](col, momo.Value("x")), "x"),
			`ARRAY["t"."a", $1] @> $2`, []any{"x", []string{"x"}}},
		{"array of one", arrayop.Contains(momo.ArrayOf[int64](momo.Value(int64(7))), 7),
			`ARRAY[$1] @> $2`, []any{int64(7), []int64{7}}},
		{"array subquery", arrayop.Contains(momo.ArraySubquery[int64](sub), 3),
			`ARRAY(SELECT "u"."id" FROM "u" WHERE "u"."n" > $1) @> $2`, []any{int64(2), []int64{3}}},
		{"array subquery of a union", arrayop.Contains(momo.ArraySubquery[int64](momo.Union(false, sub, sub)), 3),
			`ARRAY(SELECT "u"."id" FROM "u" WHERE "u"."n" > $1 UNION SELECT "u"."id" FROM "u" WHERE "u"."n" > $2) @> $3`,
			[]any{int64(2), int64(2), []int64{3}}},
	}
	for _, c := range cases {
		got, err := arrayWhere(dialect.Postgres, c.p)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if want := `SELECT COUNT(*) FROM "t" WHERE ` + c.want; got.SQL != want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, c.args) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.args)
		}
	}
}

// Catches: an empty ARRAY[] reaching PostgreSQL (which cannot type it), a nil
// subquery panicking, and arrays rendering on a dialect that has none.
func TestArrayConstructorErrors(t *testing.T) {
	cases := []struct {
		name string
		d    dialect.Name
		p    momo.Predicate
		kind error
	}{
		{"empty array", dialect.Postgres, arrayop.Contains(momo.ArrayOf[string](), "x"), momo.ErrValidation},
		{"nil subquery", dialect.Postgres, arrayop.Contains(momo.ArraySubquery[int64](nil), 1), momo.ErrValidation},
		{"array on sqlite", dialect.SQLite, arrayop.Contains(momo.ArrayOf[string](momo.Value("x")), "x"), momo.ErrUnsupported},
		{"array on mysql", dialect.MySQL, arrayop.Contains(momo.ArrayOf[string](momo.Value("x")), "x"), momo.ErrUnsupported},
		{"subquery on sqlite", dialect.SQLite, arrayop.Contains(momo.ArraySubquery[int64](arraySub(dialect.SQLite)), 1), momo.ErrUnsupported},
		{"subquery on mysql", dialect.MySQL, arrayop.Contains(momo.ArraySubquery[int64](arraySub(dialect.MySQL)), 1), momo.ErrUnsupported},
	}
	for _, c := range cases {
		if _, err := arrayWhere(c.d, c.p); !errors.Is(err, c.kind) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.kind)
		}
	}
}
