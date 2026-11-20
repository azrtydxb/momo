// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests exist to exercise the deprecated names

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/vector"
)

// sensitiveFlags is FlagSensitive in the uint16 the deprecated names take.
const sensitiveFlags = uint16(momo.FlagSensitive)

// depMasked runs `secret = <arg bound by bind>` and reports whether the hook
// saw the redaction placeholder while the driver still got the real value.
func depMasked(t *testing.T, bind func(b *momo.Builder)) bool {
	t.Helper()
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	pred := momo.PredicateFunc(func(b *momo.Builder) {
		b.Ident("secret")
		b.WriteString(" = ")
		bind(b)
	})
	if _, err := momo.Select[suite.Document](db).Where(pred).All(t.Context()); err != nil {
		t.Fatal(err)
	}
	evs, calls := rec.Events(), drv.Calls()
	if len(evs) == 0 || len(calls) == 0 {
		t.Fatal("no statement recorded")
	}
	seen, sent := evs[len(evs)-1].Args, calls[len(calls)-1].Args
	if len(seen) != 1 || len(sent) != 1 || sent[0] == momo.RedactedArg {
		t.Fatalf("hook saw %v, driver got %v", seen, sent)
	}
	return seen[0] == momo.RedactedArg
}

// Catches: the deprecated ArgFor or ExprFor forwarders dropping the sensitive
// mask (a value leaks to hooks) or masking when the flags say plain.
func TestDeprecatedArgForAndExprForMask(t *testing.T) {
	for name, bind := range map[string]func(b *momo.Builder, flags uint16){
		"ArgFor":  func(b *momo.Builder, flags uint16) { b.ArgFor(flags, "pw") },
		"ExprFor": func(b *momo.Builder, flags uint16) { b.ExprFor(flags, momo.Value("pw")) },
	} {
		for _, sensitive := range []bool{true, false} {
			flags := uint16(0)
			if sensitive {
				flags = sensitiveFlags
			}
			if got := depMasked(t, func(b *momo.Builder) { bind(b, flags) }); got != sensitive {
				t.Errorf("%s sensitive=%v: masked = %v", name, sensitive, got)
			}
		}
	}
}

// Catches: the deprecated RenderSensitive or MarkSensitive forwarders not
// reporting that an operand read a sensitive column.
func TestDeprecatedRenderAndMarkSensitive(t *testing.T) {
	d := dialect.For(dialect.Postgres)
	secret := momo.NewColumn[string]("t", "s", momo.FlagSensitive)
	plain := momo.NewColumn[string]("t", "p", 0)
	marker := momo.PredicateFunc(func(b *momo.Builder) {
		b.MarkSensitive()
		b.WriteString("x")
	})
	for name, c := range map[string]struct {
		e    momo.Expr
		want bool
	}{"sensitive column": {secret, true}, "plain column": {plain, false}, "marked expression": {marker, true}} {
		b := momo.NewBuilder(d)
		if got := b.RenderSensitive(c.e, 0)&sensitiveFlags != 0; got != c.want {
			t.Errorf("%s: sensitive = %v, want %v", name, got, c.want)
		}
	}
}

// Catches: the deprecated NewBuilder, WriteByte, AppendFilter or Unsupported
// forwarders returning a broken builder, or recording the wrong error.
func TestDeprecatedBuilderForwarders(t *testing.T) {
	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	b.WriteString("count(*)")
	if err := b.WriteByte(' '); err != nil {
		t.Fatal(err)
	}
	b.AppendFilter(momo.NewColumn[int64]("docs", "n", 0).Gt(1))
	if want := `count(*)  FILTER (WHERE "docs"."n" > $1)`; b.Err() != nil || b.String() != want {
		t.Errorf("got %q, %v; want %q", b.String(), b.Err(), want)
	}

	u := momo.NewBuilder(dialect.For(dialect.Postgres))
	u.Unsupported("thing")
	if !errors.Is(u.Err(), momo.ErrUnsupported) {
		t.Errorf("Unsupported: err = %v, want ErrUnsupported", u.Err())
	}
}

// Catches: the deprecated FlagsOf, IsBound, ValidName and ValidTypeName
// forwarders disagreeing with their documented behavior.
func TestDeprecatedHelpers(t *testing.T) {
	secret := momo.NewColumn[string]("t", "s", momo.FlagSensitive)
	if got := momo.FlagsOf(secret); got != sensitiveFlags {
		t.Errorf("FlagsOf(sensitive column) = %d, want %d", got, sensitiveFlags)
	}
	if got := momo.FlagsOf(momo.Value(1)); got != 0 {
		t.Errorf("FlagsOf(value) = %d, want 0", got)
	}
	if !momo.IsBound(momo.Value(1)) || momo.IsBound(secret) {
		t.Errorf("IsBound: value = %v, column = %v; want true, false", momo.IsBound(momo.Value(1)), momo.IsBound(secret))
	}
	if momo.ValidName("col_1") != nil || momo.ValidName("a;b") == nil {
		t.Error("ValidName accepted or rejected the wrong name")
	}
	if momo.ValidTypeName("numeric(10,2)") != nil || momo.ValidTypeName("bigint; DROP TABLE x") == nil {
		t.Error("ValidTypeName accepted or rejected the wrong name")
	}
}

// Catches: the deprecated WindowOf, VectorColumnOf or MetaTableSchema
// forwarders losing the call, the column or the table definition.
func TestDeprecatedWindowVectorAndMeta(t *testing.T) {
	tenant := momo.NewColumn[int64]("docs", "tenant", 0)
	w := momo.WindowOf[int64](momo.CountAll(), momo.Over().PartitionBy(tenant))
	got, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "docs"}).Columns(w).SQL()
	if want := `SELECT COUNT(*) OVER (PARTITION BY "docs"."tenant") FROM "docs"`; err != nil || got.SQL != want {
		t.Errorf("WindowOf: got %q, %v; want %q", got.SQL, err, want)
	}

	var src momo.VectorSource[vector.Vector] = momo.NewVectorColumn[vector.Vector]("items", "emb", 3, 0)
	if col := momo.VectorColumnOf(src); col.Table() != "items" || col.ColumnName() != "emb" || col.Dim() != 3 {
		t.Errorf("VectorColumnOf returned %s.%s dim %d", col.Table(), col.ColumnName(), col.Dim())
	}

	if tbl := momo.MetaTableSchema[suite.Document](); tbl == nil || tbl.Name != "documents" {
		t.Errorf("MetaTableSchema = %v, want the documents table", tbl)
	}
}

// Catches: the deprecated Order.AppendOrder forwarder dropping the direction.
func TestDeprecatedOrderAppendOrder(t *testing.T) {
	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	momo.NewColumn[string]("docs", "title", 0).Desc().AppendOrder(b)
	if b.String() != `"docs"."title" DESC` {
		t.Errorf("AppendOrder wrote %q", b.String())
	}
}
