// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

type mpar struct{ Tenant, Code int }
type mkid struct{ ID int }

var mkidMeta = Meta[mkid]{
	Table: "mkids", SoftDelete: -1,
	Columns: []ColumnMeta{{Name: "id"}},
	Ptr:     func(m *mkid, i int) any { return &m.ID },
}

func m2mRel() Relation[mpar, mkid] {
	return Relation[mpar, mkid]{
		Name: "Kids", Kind: ManyToMany,
		Parent: TableRef{Name: "mpars"}, Child: TableRef{Name: "mkids"},
		ChildMeta:  &mkidMeta,
		ParentCols: []string{"tenant", "code"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
		Through: &Through{Table: TableRef{Name: "mjoin"}, ParentCols: []string{"tenant", "code"}, ChildCols: []string{"kid_id"}},
		ParentKey: func(p *mpar) ([]any, bool) {
			if p.Tenant < 0 || p.Code < 0 {
				return nil, false
			}
			return []any{p.Tenant, p.Code}, true
		},
		ChildKey: func(c *mkid) ([]any, bool) { return []any{c.ID}, true },
		GetMany:  func(p *mpar) []mkid { return nil },
		SetMany:  func(p *mpar, cs []mkid) {},
	}
}

// The many-to-many statement budgets the join table's parent key columns
// (two here), not the child key width (one), and joins through the join table.
func TestLoadManyToManyChunksOnJoinKeyWidth(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 16 + 10 // 5 two-column parent tuples per statement
	var parents []mpar
	for i := 1; i <= 12; i++ {
		parents = append(parents, mpar{i, i})
	}
	parents = append(parents, mpar{-1, 1}, mpar{1, 1}) // no-key skipped, duplicate collapsed
	rel := m2mRel()
	pl, err := rel.prepareWith(loadQ{&d}, parents, &loadConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := pl.(*pending[mpar, mkid])
	if len(pd.keys) != 12 || len(pd.queries) != 3 {
		t.Fatalf("keys=%d statements=%d, want 12 and 3", len(pd.keys), len(pd.queries))
	}
	for i, q := range pd.queries {
		if len(q.Args) > 10 {
			t.Fatalf("statement %d binds %d args over the 10 available", i, len(q.Args))
		}
		if !strings.Contains(q.SQL, "JOIN") || !strings.Contains(q.SQL, "momo_key_1") {
			t.Fatalf("statement %d lacks the join or key holders: %s", i, q.SQL)
		}
	}
}

func TestLoadManyToManyNoRoomForKey(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 1
	rel := m2mRel()
	if _, err := rel.prepareWith(loadQ{&d}, []mpar{{1, 1}}, &loadConfig{}, nil); err == nil {
		t.Fatal("two key columns cannot fit in one parameter")
	}
}
