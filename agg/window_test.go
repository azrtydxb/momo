// SPDX-License-Identifier: Apache-2.0

package agg_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
)

func TestAggExprOverGolden(t *testing.T) {
	pg := exprtest.DB(dialect.Postgres)
	sq := exprtest.DB(dialect.SQLite)
	part := momo.Over().PartitionBy(suite.DocTenantID)

	cases := []struct {
		name string
		db   *momo.DB
		e    momo.Expr
		want string
		args []any
	}{
		{"filter then over", pg,
			agg.String(suite.DocTitle, ",").Filter(suite.DocTitle.Eq("x")).Over(part),
			`SELECT string_agg("documents"."title", ',') FILTER (WHERE "documents"."title" = $1) OVER (PARTITION BY "documents"."tenant_id") FROM "documents"`,
			[]any{"x"}},
		{"generic aggregate with an ordered frame", pg,
			agg.Func[int64]("max", suite.DocVersion).Over(momo.Over().OrderBy(suite.DocID.Asc()).Rows(momo.Preceding(1), momo.CurrentRow())),
			`SELECT max("documents"."version") OVER (ORDER BY "documents"."id" ASC ROWS BETWEEN 1 PRECEDING AND CURRENT ROW) FROM "documents"`,
			nil},
		{"json aggregate on sqlite", sq,
			agg.JSONB[string](suite.DocTitle).Over(part),
			`SELECT json_group_array("documents"."title") OVER (PARTITION BY "documents"."tenant_id") FROM "documents"`,
			nil},
	}
	for _, c := range cases {
		got, err := momo.Select[exprtest.Tally](c.db).From(momo.TableRef{Name: "documents"}).Columns(c.e).SQL()
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
	pg := exprtest.DB(dialect.Postgres)
	my := exprtest.DB(dialect.MySQL)
	part := momo.Over().PartitionBy(suite.DocTenantID)
	build := func(d *momo.DB, e momo.Expr) error {
		_, err := momo.Select[exprtest.Tally](d).From(momo.TableRef{Name: "documents"}).Columns(e).SQL()
		return err
	}
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"within group", build(pg, agg.PercentileCont(0.5).WithinGroup(suite.DocID.Asc()).Over(part)), momo.ErrValidation},
		{"order by inside the call", build(pg, agg.String(suite.DocTitle, ",").OrderBy(suite.DocID.Asc()).Over(part)), momo.ErrValidation},
		{"filter without predicate", build(pg, agg.String(suite.DocTitle, ",").Filter(nil).Over(part)), momo.ErrValidation},
		{"json array filter on mysql", build(my, agg.JSONB[int64](suite.DocID).Filter(suite.DocTitle.Eq("x")).Over(part)), momo.ErrUnsupported},
		{"unknown generic filter on mysql", build(my, agg.Func[int64]("json_arrayagg", suite.DocID).Filter(suite.DocTitle.Eq("x")).Over(part)), momo.ErrUnsupported},
		{"filter without predicate on mysql", build(my, agg.String(suite.DocTitle, ",").Filter(nil).Over(part)), momo.ErrValidation},
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
	call := agg.String(suite.DocTitle, ",").Filter(suite.DocSecret.Eq(sensitiveValue)).
		Over(momo.Over().PartitionBy(suite.DocTenantID))
	if _, err := momo.Select[ktRow](db).From(momo.TableRef{Name: "documents"}).Columns(suite.DocTenantID, call).All(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertMasked(t, rec, drv)
}

const sensitiveValue = "hunter2-sensitive"

// ktRow is a key and a title, the two columns the masking test selects.
type ktRow struct {
	K int64
	T string
}

var ktRowMeta = momo.Meta[ktRow]{
	Columns:  []momo.ColumnMeta{{Name: "k"}, {Name: "t"}},
	Ptr:      func(m *ktRow, i int) any { return [...]any{&m.K, &m.T}[i] },
	Value:    func(m *ktRow, i int) any { return [...]any{m.K, m.T}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*ktRow) MomoMeta() *momo.Meta[ktRow] { return &ktRowMeta }

// assertMasked checks that no hook event shows the sensitive value, at least
// one shows the placeholder, and the driver received the real value.
func assertMasked(t *testing.T, rec *obstest.Recorder, drv *fakedriver.Driver) {
	t.Helper()
	masked := false
	for _, e := range rec.Events() {
		if e.NumArgs != len(e.Args) {
			t.Errorf("NumArgs %d != len(Args) %d", e.NumArgs, len(e.Args))
		}
		for _, a := range e.Args {
			if a == sensitiveValue {
				t.Fatalf("hook saw the sensitive value in %q", e.SQL)
			}
			if a == momo.RedactedArg {
				masked = true
			}
		}
	}
	if !masked {
		t.Fatal("no hook event carried the redaction placeholder")
	}
	sent := false
	for _, c := range drv.Calls() {
		if slices.Contains(c.Args, any(sensitiveValue)) {
			sent = true
		}
	}
	if !sent {
		t.Fatal("the driver did not receive the real value")
	}
}
