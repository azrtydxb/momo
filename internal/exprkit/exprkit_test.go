// SPDX-License-Identifier: Apache-2.0

package exprkit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprkit"
	"github.com/azrtydxb/momo/internal/suite"
)

// Catches: a function of the bridge that the root forgot to register, or that is
// wired to the wrong root internal, which would panic or misbehave only when a
// feature package first reaches it. Every exprkit function runs once.
func TestEveryBridgeFunctionWorks(t *testing.T) {
	secret := momo.NewColumn[string]("docs", "secret", momo.FlagSensitive)
	plain := momo.NewColumn[string]("docs", "title", 0)
	count := momo.NewColumn[int64]("docs", "n", 0)
	pg := dialect.For(dialect.Postgres)

	b := exprkit.NewBuilder(pg)
	exprkit.ArgFor(b, momo.FlagSensitive, "pw")
	if got := b.Args(); len(got) != 1 || got[0] != "pw" {
		t.Errorf("ArgFor args = %v", got)
	}
	if fl := exprkit.RenderSensitive(b, secret, 0); fl&momo.FlagSensitive == 0 {
		t.Errorf("RenderSensitive of a sensitive column = %v", fl)
	}
	if fl := exprkit.RenderSensitive(b, plain, 0); fl&momo.FlagSensitive != 0 {
		t.Errorf("RenderSensitive of a plain column = %v", fl)
	}
	exprkit.ExprFor(b, momo.FlagSensitive, momo.Value("x"))
	exprkit.MarkSensitive(b)

	f := exprkit.NewBuilder(pg)
	f.WriteString("COUNT(*)")
	exprkit.AppendFilter(f, count.Gt(1))
	if !strings.Contains(f.String(), " FILTER (WHERE ") {
		t.Errorf("AppendFilter wrote %q", f.String())
	}

	u := exprkit.NewBuilder(pg)
	exprkit.Unsupported(u, "thing")
	if !errors.Is(u.Err(), momo.ErrUnsupported) {
		t.Errorf("Unsupported err = %v", u.Err())
	}

	if exprkit.FlagsOf(secret)&momo.FlagSensitive == 0 || exprkit.FlagsOf(momo.Value(1)) != 0 {
		t.Error("FlagsOf did not read the flags")
	}
	if !exprkit.IsBound(momo.Value(1)) || exprkit.IsBound(plain) || exprkit.IsBound(nil) {
		t.Error("IsBound is wrong")
	}

	p := exprkit.NewBuilder(pg)
	exprkit.Predicate(func(b *momo.Builder) { b.WriteString("1 = 1") }).AppendSQL(p)
	if p.String() != "1 = 1" {
		t.Errorf("Predicate wrote %q", p.String())
	}

	if exprkit.ValidName("col_1") != nil || exprkit.ValidName("a;b") == nil {
		t.Error("ValidName is wrong")
	}
	if exprkit.ValidTypeName("numeric(10,2)") != nil || exprkit.ValidTypeName("x; y") == nil {
		t.Error("ValidTypeName is wrong")
	}

	w := exprkit.NewBuilder(pg)
	exprkit.WindowCall(momo.CountAll(), momo.Over()).AppendSQL(w)
	if !strings.Contains(w.String(), "OVER") {
		t.Errorf("WindowCall wrote %q", w.String())
	}

	o := exprkit.NewBuilder(pg)
	exprkit.AppendOrder(o, plain.Desc())
	if o.String() != `"docs"."title" DESC` {
		t.Errorf("AppendOrder wrote %q", o.String())
	}

	if exprkit.TableSchema[suite.Document]() == nil {
		t.Error("TableSchema of a model is nil")
	}
}
