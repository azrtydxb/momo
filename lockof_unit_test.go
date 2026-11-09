// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestLockOfGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	my := momo.New(fakedriver.New(dialect.MySQL))
	sq := momo.New(fakedriver.New(dialect.SQLite))

	d := momo.TableRef{Name: "documents", Alias: "d"}
	other := momo.TableRef{Name: "tenants", Alias: `t"x`}
	join := func(db *momo.DB) momo.SelectQuery[unitDocument] {
		return momo.Select[unitDocument](db).From(d).Join(other, DocTenantID.As("d").EqExpr(DocID.As(`t"x`)))
	}

	cases := []struct {
		name string
		q    momo.SelectQuery[unitDocument]
		tail string
	}{
		{"update of skip locked", join(pg).ForUpdate("SKIP LOCKED").LockOf("d"), ` FOR UPDATE OF "d" SKIP LOCKED`},
		{"order independent", join(pg).LockOf("d").ForUpdate("SKIP LOCKED"), ` FOR UPDATE OF "d" SKIP LOCKED`},
		{"share of nowait two tables", join(pg).ForShare("NOWAIT").LockOf("d", `t"x`), ` FOR SHARE OF "d", "t""x" NOWAIT`},
		{"update of only", join(pg).ForUpdate().LockOf("d"), ` FOR UPDATE OF "d"`},
		{"mysql", join(my).ForUpdate("SKIP LOCKED").LockOf("d"), " FOR UPDATE OF `d` SKIP LOCKED"},
	}
	for _, c := range cases {
		got, err := c.q.SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(got.SQL) < len(c.tail) || got.SQL[len(got.SQL)-len(c.tail):] != c.tail {
			t.Errorf("%s: SQL %q does not end with %q", c.name, got.SQL, c.tail)
		}
	}

	for name, err := range map[string]error{
		"sqlite":                    buildErr(join(sq).ForUpdate().LockOf("d")),
		"no lock":                   buildErr(join(pg).LockOf("d")),
		"empty":                     buildErr(join(pg).ForUpdate().LockOf()),
		"blank":                     buildErr(join(pg).ForUpdate().LockOf("")),
		"unknown":                   buildErr(join(pg).ForUpdate().LockOf("nope")),
		"model name, not the alias": buildErr(join(pg).ForUpdate().LockOf("documents")),
		"subquery":                  buildErr(momo.Select[unitDocument](pg).From(momo.FromSubquery(momo.RawSubquery("SELECT 1"), "c")).ForUpdate().LockOf("c")),
		"distinct":                  buildErr(join(pg).Distinct().ForUpdate().LockOf("d")),
		"group by":                  buildErr(join(pg).GroupBy(DocTenantID.As("d")).ForUpdate()),
		"both wait options":         buildErr(join(pg).ForUpdate("NOWAIT", "SKIP LOCKED")),
	} {
		want := momo.ErrValidation
		if name == "sqlite" {
			want = momo.ErrUnsupported
		}
		if !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", name, err, want)
		}
	}

	// A later ForUpdate without options clears an earlier wait mode.
	if c, err := join(pg).ForUpdate("NOWAIT").ForUpdate().SQL(); err != nil || strings.HasSuffix(c.SQL, "NOWAIT") {
		t.Errorf("later ForUpdate kept the wait mode: %q, %v", c.SQL, err)
	}

	// Immutability: LockOf must not change the base query.
	base := join(pg).ForUpdate()
	_ = base.LockOf("d")
	if c, _ := base.SQL(); c.SQL[len(c.SQL)-len(" FOR UPDATE"):] != " FOR UPDATE" {
		t.Errorf("base query mutated: %s", c.SQL)
	}
}

func buildErr(q momo.SelectQuery[unitDocument]) error {
	_, err := q.SQL()
	return err
}

func TestLockOfSourcesAndRestrictions(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	base := func() momo.SelectQuery[unitDocument] { return momo.Select[unitDocument](pg) }
	cteJoin := func() momo.SelectQuery[unitDocument] {
		return base().With("c", momo.RawSubquery("SELECT 1 AS id"), "id").
			Join(momo.CTE("c"), DocID.EqExpr(momo.ColumnOf[int64]("c", "id")))
	}

	// Unaliased schema-qualified source: the lock names the bare table.
	schema := base().From(momo.TableRef{Name: "documents", Schema: "tenant"}).ForUpdate("NOWAIT").LockOf("documents")
	if c, err := schema.SQL(); err != nil || !strings.HasSuffix(c.SQL, ` FOR UPDATE OF "documents" NOWAIT`) {
		t.Errorf("schema-qualified: %q, %v", c.SQL, err)
	}
	// The same option twice is allowed.
	if c, err := base().ForUpdate("NOWAIT", "NOWAIT").SQL(); err != nil || !strings.HasSuffix(c.SQL, " FOR UPDATE NOWAIT") {
		t.Errorf("repeated NOWAIT: %q, %v", c.SQL, err)
	}
	// The base table of a query with a CTE join stays lockable.
	if c, err := cteJoin().ForUpdate().LockOf("documents").SQL(); err != nil || !strings.HasSuffix(c.SQL, ` FOR UPDATE OF "documents"`) {
		t.Errorf("lock the base table next to a CTE: %q, %v", c.SQL, err)
	}

	for name, q := range map[string]momo.SelectQuery[unitDocument]{
		"CTE name":         cteJoin().ForUpdate().LockOf("c"),
		"having":           base().GroupBy(DocTenantID).Having(DocTenantID.Eq(1)).ForUpdate(),
		"share + distinct": base().Distinct().ForShare(),
		"share + group by": base().GroupBy(DocTenantID).ForShare("SKIP LOCKED"),
	} {
		if err := buildErr(q); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}
