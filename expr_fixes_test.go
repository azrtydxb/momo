// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestDistinctFromPerDialect(t *testing.T) {
	cases := []struct {
		d    dialect.Name
		p    momo.Predicate
		want string
	}{
		{dialect.Postgres, DocTitle.IsDistinctFrom("a"), `"documents"."title" IS DISTINCT FROM $1`},
		{dialect.Postgres, DocTitle.IsNotDistinctFrom("a"), `"documents"."title" IS NOT DISTINCT FROM $1`},
		{dialect.SQLite, DocTitle.IsDistinctFrom("a"), `"documents"."title" IS NOT ?`},
		{dialect.MySQL, DocTitle.IsDistinctFrom("a"), "NOT (`documents`.`title` <=> ?)"},
		{dialect.MySQL, DocTitle.IsNotDistinctFrom("a"), "`documents`.`title` <=> ?"},
	}
	for _, c := range cases {
		db := momo.New(fakedriver.New(c.d))
		got, err := momo.Select[unitDocument](db).Where(c.p).SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.d, err)
		}
		if !strings.Contains(got.SQL, c.want) {
			t.Errorf("%s: got %s, want it to contain %s", c.d, got.SQL, c.want)
		}
	}
}

// Variadic arguments are copied on store, so a caller reusing its slice
// cannot change a query built earlier.
func TestVariadicSlicesAreCloned(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	cases := []struct {
		name string
		mk   func() (q momo.SelectQuery[unitDocument], mutate func())
	}{
		{"In", func() (momo.SelectQuery[unitDocument], func()) {
			vs := []int64{1, 2}
			return momo.Select[unitDocument](pg).Where(DocTenantID.In(vs...)), func() { vs[0] = 99 }
		}},
		{"And", func() (momo.SelectQuery[unitDocument], func()) {
			ps := []momo.Predicate{DocTenantID.Eq(1), DocTenantID.Eq(2)}
			return momo.Select[unitDocument](pg).Where(momo.And(ps...)), func() { ps[0] = DocTenantID.Eq(99) }
		}},
		{"Or", func() (momo.SelectQuery[unitDocument], func()) {
			ps := []momo.Predicate{DocTenantID.Eq(1), DocTenantID.Eq(2)}
			return momo.Select[unitDocument](pg).Where(momo.Or(ps...)), func() { ps[0] = DocTenantID.Eq(99) }
		}},
		{"SQL", func() (momo.SelectQuery[unitDocument], func()) {
			args := []any{int64(1)}
			return momo.Select[unitDocument](pg).Where(momo.SQL("a = ?", args...)), func() { args[0] = int64(99) }
		}},
		{"TypedSQL", func() (momo.SelectQuery[unitDocument], func()) {
			args := []any{int64(1)}
			return momo.Select[unitDocument](pg).Where(momo.TypedSQL[bool]("a = ?", args...)), func() { args[0] = int64(99) }
		}},
		{"Union", func() (momo.SelectQuery[unitDocument], func()) {
			qs := []momo.Subquery{momo.Select[unitDocument](pg), momo.Select[unitDocument](pg)}
			return momo.Select[unitDocument](pg).From(momo.FromSubquery(momo.Union(true, qs...), "u")), func() { qs[1] = momo.Select[unitDocument](pg).Where(DocTenantID.Eq(99)) }
		}},
		{"With", func() (momo.SelectQuery[unitDocument], func()) {
			cols := []string{"x"}
			return momo.Select[unitDocument](pg).With("c", momo.Select[unitDocument](pg), cols...), func() { cols[0] = "mutated" }
		}},
	}
	for _, c := range cases {
		q, mutate := c.mk()
		want, err := q.SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		mutate()
		got, err := q.SQL()
		if err != nil {
			t.Fatalf("%s after mutation: %v", c.name, err)
		}
		if got.SQL != want.SQL || fmt.Sprint(got.Args) != fmt.Sprint(want.Args) {
			t.Errorf("%s: query changed after the caller mutated its slice:\n got %s %v\nwant %s %v", c.name, got.SQL, got.Args, want.SQL, want.Args)
		}
	}
}
