// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// ckid and cpar model a two-column key; a negative component means NULL.
type ckid struct{ Tenant, Code, Seq int }
type cpar struct{ Tenant, Code int }

var ckidMeta = Meta[ckid]{
	Table:      "ckids",
	SoftDelete: -1,
	Columns:    []ColumnMeta{{Name: "tenant"}, {Name: "code"}, {Name: "seq"}},
	Ptr: func(m *ckid, i int) any {
		return []any{&m.Tenant, &m.Code, &m.Seq}[i]
	},
}

func ckidsRel() Relation[cpar, ckid] {
	return Relation[cpar, ckid]{
		Name: "Kids", Kind: HasMany,
		Parent: TableRef{Name: "cpars"}, Child: TableRef{Name: "ckids"},
		ChildMeta:  &ckidMeta,
		ParentCols: []string{"tenant", "code"}, ChildCols: []string{"tenant", "code"}, ChildPK: []string{"seq"},
		// An extractor that reports ok=true with a NULL element: the loader must
		// still treat the key as absent.
		ParentKey: func(p *cpar) ([]any, bool) {
			var t, c any
			if p.Tenant >= 0 {
				t = p.Tenant
			}
			if p.Code >= 0 {
				c = p.Code
			}
			return []any{t, c}, true
		},
		ChildKey: func(c *ckid) ([]any, bool) { return []any{c.Tenant, c.Code}, true },
		GetMany:  func(p *cpar) []ckid { return nil },
		SetMany:  func(p *cpar, cs []ckid) {},
	}
}

// Composite keys: duplicate parent keys collapse to one tuple, a NULL in any
// component drops the key, and chunks respect the bind limit in key order.
func TestLoadCompositeKeysDedupeNullAndChunk(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 16 + 10 // 16 reserved leaves 10 binds: 5 two-column tuples per statement
	parents := []cpar{{1, 1}, {1, 1}, {-1, 2}, {2, -1}}
	for i := 3; i <= 14; i++ {
		parents = append(parents, cpar{i, i})
	}
	rel := ckidsRel()
	pl, err := rel.prepareWith(loadQ{&d}, parents, &loadConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := pl.(*pending[cpar, ckid])
	// Distinct non-NULL keys: (1,1) and (3,3)..(14,14) = 13 -> 3 statements of at most 5.
	if len(pd.keys) != 13 || len(pd.queries) != 3 {
		t.Fatalf("keys=%d statements=%d, want 13 and 3", len(pd.keys), len(pd.queries))
	}
	var args []any
	for i, q := range pd.queries {
		if len(q.Args) > 10 {
			t.Fatalf("statement %d binds %d key args, over the 10 available", i, len(q.Args))
		}
		args = append(args, q.Args...)
	}
	if len(args) != 26 || args[0] != 1 || args[1] != 1 || args[2] != 3 || args[25] != 14 {
		t.Fatalf("tuples lost or reordered across chunks: %v", args)
	}
}
