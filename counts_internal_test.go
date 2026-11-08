// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
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

// A polymorphic relation adds the discriminator predicate and one argument.
func TestCountQueriesPolymorphicDiscriminator(t *testing.T) {
	rel := ckidsRel()
	rel.Discriminator = &Discriminator{Column: "kind", Value: "doc"}
	qs, err := rel.countQueries(dialect.For(dialect.Postgres), [][]any{{1, 1}}, &loadConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(qs[0].SQL, `"ckids"."kind" = $3`) || !strings.HasSuffix(qs[0].SQL, `GROUP BY "ckids"."tenant", "ckids"."code"`) {
		t.Fatalf("discriminator predicate missing: %s", qs[0].SQL)
	}
	if len(qs[0].Args) != 3 || qs[0].Args[2] != "doc" {
		t.Fatalf("args %v, want the two key parts and the discriminator value", qs[0].Args)
	}
}

func TestCountQueriesHasOneGolden(t *testing.T) {
	rel := ckidsRel()
	rel.Kind = HasOne
	rel.GetOne = func(p *cpar) *ckid { return nil }
	rel.SetOne = func(p *cpar, c *ckid) {}
	if err := rel.validate(); err != nil {
		t.Fatal(err)
	}
	qs, err := rel.countQueries(dialect.For(dialect.Postgres), [][]any{{1, 1}}, &loadConfig{})
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "ckids"."tenant", "ckids"."code", COUNT(*) FROM "ckids" WHERE ("ckids"."tenant", "ckids"."code") IN (($1, $2)) GROUP BY "ckids"."tenant", "ckids"."code"`
	if qs[0].SQL != want {
		t.Fatalf("got %s\nwant %s", qs[0].SQL, want)
	}
}

// A filter that binds an argument shrinks the key budget, so no chunk goes
// over the bind limit.
func TestCountQueriesFilterArgsStayUnderBindLimit(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 26
	var keys [][]any
	for i := 1; i <= 12; i++ {
		keys = append(keys, []any{i, i})
	}
	cfg := &loadConfig{where: []Predicate{binOp{qcol{"ckids", "seq"}, " = ", argExpr{v: 5}}}}
	rel := ckidsRel()
	qs, err := rel.countQueries(&d, keys, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 3 { // 26 - 16 reserved - 1 filter = 9 binds: 4 two-column tuples
		t.Fatalf("got %d statements, want 3", len(qs))
	}
	for i, q := range qs {
		if len(q.Args) > d.MaxParams {
			t.Fatalf("statement %d binds %d args over the limit %d", i, len(q.Args), d.MaxParams)
		}
	}
}

// The key arguments of a count statement are marked sensitive on the side that
// holds the key, so hooks see the placeholder.
func TestCountQueriesMarkSensitiveKeys(t *testing.T) {
	for name, kind := range map[string]RelationKind{"has many child key": HasMany, "many to many parent key": ManyToMany} {
		t.Run(name, func(t *testing.T) {
			r := secRelation(kind)
			qs, err := r.countQueries(dialect.For(dialect.Postgres), [][]any{{relSecret}}, &loadConfig{})
			if err != nil || len(qs) != 1 {
				t.Fatalf("countQueries: %v %d", err, len(qs))
			}
			masked := maskArgs(qs[0].Args, qs[0].sens)
			if len(masked) != 1 || masked[0] != RedactedArg {
				t.Fatalf("hook args %v, want [%s]", masked, RedactedArg)
			}
			if qs[0].Args[0] != relSecret {
				t.Fatalf("the driver must still get the key: %v", qs[0].Args)
			}
		})
	}
}

func manyArgFilters(n int) *loadConfig {
	cfg := &loadConfig{}
	for i := 0; i < n; i++ {
		cfg.where = append(cfg.where, binOp{qcol{"ckids", "seq"}, " = ", argExpr{v: i}})
	}
	return cfg
}

// A filter binding more than the 16 reserved arguments still shrinks the key
// budget, so a chunk never goes over the bind limit.
func TestCountQueriesManyFilterArgsShrinkTheKeyBudget(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 16 + 20 + 10 // 20 filter binds leave room for five two-column keys
	var keys [][]any
	for i := 1; i <= 12; i++ {
		keys = append(keys, []any{i, i})
	}
	qs, err := ckidsRel().countQueries(&d, keys, manyArgFilters(20))
	if err != nil {
		t.Fatal(err)
	}
	// Ignoring the filter binds would fit all twelve keys in one statement.
	if len(qs) != 3 {
		t.Fatalf("got %d statements, want 3 (five keys each)", len(qs))
	}
	for i, q := range qs {
		if len(q.Args) > d.MaxParams {
			t.Fatalf("statement %d binds %d args over the limit %d", i, len(q.Args), d.MaxParams)
		}
	}
}

// A scope that leaves no room for a single key is a validation error.
func TestCountQueriesScopeLeavesNoRoomForKey(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 21 // 20 filter binds plus a two-column key is 22
	_, err := ckidsRel().countQueries(&d, [][]any{{1, 1}}, manyArgFilters(20))
	var me *Error
	if !errors.As(err, &me) || me.Kind != ErrValidation || !strings.Contains(err.Error(), "no room for a key") {
		t.Fatalf("err = %v, want ErrValidation naming the missing key room", err)
	}
}

// Only the side that holds the parent key decides whether the key is masked:
// the child join column for has-many, the parent key column for many-to-many.
func TestCountQueriesMaskOnlyTheKeyHoldingSide(t *testing.T) {
	cases := []struct {
		name                  string
		kind                  RelationKind
		parentFlag, childFlag uint16
		masked                bool
	}{
		{"has many sensitive child", HasMany, 0, FlagSensitive, true},
		{"has many sensitive parent only", HasMany, FlagSensitive, 0, false},
		{"many to many sensitive parent", ManyToMany, FlagSensitive, 0, true},
		{"many to many sensitive child only", ManyToMany, 0, FlagSensitive, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := secRelationWith(c.kind, c.parentFlag, c.childFlag)
			qs, err := r.countQueries(dialect.For(dialect.Postgres), [][]any{{relSecret}}, &loadConfig{})
			if err != nil || len(qs) != 1 {
				t.Fatalf("countQueries: %v %d", err, len(qs))
			}
			got := maskArgs(qs[0].Args, qs[0].sens)
			want := any(relSecret)
			if c.masked {
				want = RedactedArg
			}
			if len(got) != 1 || got[0] != want {
				t.Fatalf("hook args %v, want [%v]", got, want)
			}
		})
	}
}
