// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
)

func TestAggExprOverGolden(t *testing.T) {
	pg := typedDB(dialect.Postgres)
	sq := typedDB(dialect.SQLite)
	part := momo.Over().PartitionBy(DocTenantID)

	cases := []struct {
		name string
		db   *momo.DB
		e    momo.Expr
		want string
		args []any
	}{
		{"filter then over", pg,
			momo.StringAgg(DocTitle, ",").Filter(DocTitle.Eq("x")).Over(part),
			`SELECT string_agg("documents"."title", ',') FILTER (WHERE "documents"."title" = $1) OVER (PARTITION BY "documents"."tenant_id") FROM "documents"`,
			[]any{"x"}},
		{"generic aggregate with an ordered frame", pg,
			momo.Aggregate[int64]("max", anVersion).Over(momo.Over().OrderBy(DocID.Asc()).Rows(momo.Preceding(1), momo.CurrentRow())),
			`SELECT max("documents"."version") OVER (ORDER BY "documents"."id" ASC ROWS BETWEEN 1 PRECEDING AND CURRENT ROW) FROM "documents"`,
			nil},
		{"json aggregate on sqlite", sq,
			momo.JSONBAgg[string](DocTitle).Over(part),
			`SELECT json_group_array("documents"."title") OVER (PARTITION BY "documents"."tenant_id") FROM "documents"`,
			nil},
	}
	for _, c := range cases {
		got, err := momo.Select[anN](c.db).From(anDocs).Columns(c.e).SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
		if len(got.Args) != len(c.args) || (len(c.args) > 0 && got.Args[0] != c.args[0]) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.args)
		}
	}
}

func TestAggExprOverErrors(t *testing.T) {
	pg := typedDB(dialect.Postgres)
	my := typedDB(dialect.MySQL)
	part := momo.Over().PartitionBy(DocTenantID)
	build := func(d *momo.DB, e momo.Expr) error {
		_, err := momo.Select[anN](d).From(anDocs).Columns(e).SQL()
		return err
	}
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"within group", build(pg, momo.PercentileCont(0.5).WithinGroup(DocID.Asc()).Over(part)), momo.ErrValidation},
		{"order by inside the call", build(pg, momo.StringAgg(DocTitle, ",").OrderBy(DocID.Asc()).Over(part)), momo.ErrValidation},
		{"filter without predicate", build(pg, momo.StringAgg(DocTitle, ",").Filter(nil).Over(part)), momo.ErrValidation},
		{"filter on mysql", build(my, momo.StringAgg(DocTitle, ",").Filter(DocTitle.Eq("x")).Over(part)), momo.ErrUnsupported},
	}
	for _, c := range cases {
		if !errors.Is(c.err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, c.err, c.want)
		}
	}
}

// TestAggExprOverMasksSensitiveFilter pins that a sensitive value compared
// inside the FILTER of a windowed aggregate is masked for hooks.
func TestAggExprOverMasksSensitiveFilter(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	agg := momo.StringAgg(suite.DocTitle, ",").Filter(suite.DocSecret.Eq(sensitiveValue)).
		Over(momo.Over().PartitionBy(suite.DocTenantID))
	if _, err := momo.Select[anKT](db).From(anDocs).Columns(suite.DocTenantID, agg).All(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertMasked(t, rec, drv)
}
