// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: COALESCE over expressions dropping an argument, CAST rendered with
// the wrong type text, and concat using the wrong operator per dialect.
func TestCoalesceCastConcatGolden(t *testing.T) {
	cases := []struct {
		d    dialect.Name
		e    momo.Expr
		want string
	}{
		{dialect.Postgres, momo.CoalesceExpr(DocTitle, momo.Lower(DocTitle)), `coalesce("documents"."title", lower("documents"."title"))`},
		{dialect.SQLite, momo.CoalesceExpr(DocTitle, DocTitle, momo.Upper(DocTitle)), `coalesce("documents"."title", "documents"."title", upper("documents"."title"))`},
		{dialect.Postgres, momo.Cast[int64](momo.Sum(DocTenantID), "bigint"), `CAST(SUM("documents"."tenant_id") AS bigint)`},
		{dialect.Postgres, momo.Cast[float64](DocTenantID, "numeric(10,2)").As("v"), `CAST("documents"."tenant_id" AS numeric(10,2)) AS "v"`},
		{dialect.Postgres, momo.Cast[float64](DocTenantID, "double precision"), `CAST("documents"."tenant_id" AS double precision)`},
		{dialect.MySQL, momo.Cast[int64](DocTenantID, "signed"), "CAST(`documents`.`tenant_id` AS signed)"},
		{dialect.Postgres, momo.Concat(DocTitle, momo.Value("-x"), DocTitle), `("documents"."title" || $1 || "documents"."title")`},
		{dialect.SQLite, momo.Concat(DocTitle, DocTitle), `("documents"."title" || "documents"."title")`},
		{dialect.MySQL, momo.Concat(DocTitle, DocTitle), "CONCAT(`documents`.`title`, `documents`.`title`)"},
	}
	for _, c := range cases {
		got, err := momo.Select[unitTally](typedDB(c.d)).From(momo.TableRef{Name: "documents"}).Columns(c.e).SQL()
		if err != nil {
			t.Errorf("%s %s: %v", c.d, c.want, err)
			continue
		}
		if want := quoteFor(c.d, "SELECT "+c.want+` FROM "documents"`); got.SQL != want {
			t.Errorf("%s:\n got %s\nwant %s", c.d, got.SQL, want)
		}
	}
}

// Catches: a CAST type name reaching the SQL text unchecked.
func TestCastTypeValidation(t *testing.T) {
	for _, bad := range []string{"", "bigint; DROP TABLE x", "big)int", "a(", "numeric(x)", "numeric()", "x--", "a  b", "int(1,)", "(10)"} {
		_, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
			Columns(momo.Cast[int64](DocTenantID, bad)).SQL()
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("type %q err = %v, want ErrValidation", bad, err)
		}
	}
}
