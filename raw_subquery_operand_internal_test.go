// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// A raw scalar subquery works as a typed comparison operand through
// TypedSQL("?", sub) (a Subquery argument renders its own parentheses): its '?' args bind in order, numbered per dialect.
// Would fail if the subquery's args were dropped or mis-numbered.
func TestRawSubqueryAsTypedComparisonOperand(t *testing.T) {
	version := NewColumn[int64]("documents", "version", 0)
	sub := RawSubquery(`SELECT MAX(version) FROM documents WHERE tenant_id = ?`, int64(7))
	q := Select[scopeRow](nil).From(TableRef{Name: "documents"}).Columns(version).
		Where(version.GtExpr(TypedSQL[int64]("?", sub)), version.Lt(100))

	for name, want := range map[dialect.Name]string{
		dialect.Postgres: `SELECT "documents"."version" FROM "documents" WHERE ("documents"."version" > (SELECT MAX(version) FROM documents WHERE tenant_id = $1)) AND ("documents"."version" < $2)`,
		dialect.SQLite:   `SELECT "documents"."version" FROM "documents" WHERE ("documents"."version" > (SELECT MAX(version) FROM documents WHERE tenant_id = ?)) AND ("documents"."version" < ?)`,
	} {
		c, err := compile(dialect.For(name), q.render)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c.SQL != want {
			t.Errorf("%s: sql = %s, want %s", name, c.SQL, want)
		}
		if !reflect.DeepEqual(c.Args, []any{int64(7), int64(100)}) {
			t.Errorf("%s: args = %v, want [7 100]", name, c.Args)
		}
	}
}
