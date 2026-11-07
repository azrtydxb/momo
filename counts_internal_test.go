// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestCountQueriesGolden(t *testing.T) {
	ck := ckidsRel()
	mm := m2mRel()
	cases := []struct {
		name string
		sql  func() (Compiled, error)
		want string
	}{
		{"composite has-many postgres", func() (Compiled, error) {
			qs, err := ck.countQueries(dialect.For(dialect.Postgres), [][]any{{1, 1}, {2, 2}}, &loadConfig{})
			return qs[0], err
		}, `SELECT "ckids"."tenant", "ckids"."code", COUNT(*) FROM "ckids" WHERE ("ckids"."tenant", "ckids"."code") IN (($1, $2), ($3, $4)) GROUP BY "ckids"."tenant", "ckids"."code"`},
		{"composite has-many sqlite", func() (Compiled, error) {
			qs, err := ck.countQueries(dialect.For(dialect.SQLite), [][]any{{1, 1}, {2, 2}}, &loadConfig{})
			return qs[0], err
		}, `SELECT "ckids"."tenant", "ckids"."code", COUNT(*) FROM "ckids" WHERE (("ckids"."tenant" = ? AND "ckids"."code" = ?) OR ("ckids"."tenant" = ? AND "ckids"."code" = ?)) GROUP BY "ckids"."tenant", "ckids"."code"`},
		{"many-to-many counts the join table alone", func() (Compiled, error) {
			qs, err := mm.countQueries(dialect.For(dialect.Postgres), [][]any{{1, 1}}, &loadConfig{})
			return qs[0], err
		}, `SELECT "mjoin"."tenant", "mjoin"."code", COUNT(*) FROM "mjoin" WHERE ("mjoin"."tenant", "mjoin"."code") IN (($1, $2)) GROUP BY "mjoin"."tenant", "mjoin"."code"`},
	}
	for _, c := range cases {
		got, err := c.sql()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
	}
}

// A filter on a many-to-many child needs the join to the far table.
func TestCountQueriesManyToManyFilterJoins(t *testing.T) {
	mm := m2mRel()
	cfg := &loadConfig{where: []Predicate{binOp{qcol{"mkids", "id"}, " = ", argExpr{v: 5}}}}
	qs, err := mm.countQueries(dialect.For(dialect.Postgres), [][]any{{1, 1}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(qs[0].SQL, `INNER JOIN "mkids" ON "mjoin"."kid_id" = "mkids"."id"`) || !strings.HasSuffix(qs[0].SQL, `GROUP BY "mjoin"."tenant", "mjoin"."code"`) {
		t.Fatalf("filtered many-to-many count lacks the join: %s", qs[0].SQL)
	}
	if len(qs[0].Args) != 3 {
		t.Fatalf("args %v, want the two key parts and the filter value", qs[0].Args)
	}
}

func TestCountQueriesChunkOnBindLimit(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 16 + 10 // five two-column tuples per statement
	var keys [][]any
	for i := 1; i <= 12; i++ {
		keys = append(keys, []any{i, i})
	}
	rel := ckidsRel()
	qs, err := rel.countQueries(&d, keys, &loadConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 3 {
		t.Fatalf("12 keys at 5 per statement need 3 statements, got %d", len(qs))
	}
	for i, q := range qs {
		if len(q.Args) > 10 {
			t.Fatalf("statement %d binds %d args", i, len(q.Args))
		}
	}
}
