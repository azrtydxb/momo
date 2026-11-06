// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// limitedQuerier reports a dialect with a tiny bind limit and never executes.
type limitedQuerier struct {
	Querier
	d *dialect.Dialect
}

func (l limitedQuerier) Dialect() *dialect.Dialect { return l.d }

type assocParent struct{ ID int64 }
type assocChild struct{ ID int64 }

func assocRelation(kind RelationKind) Relation[assocParent, assocChild] {
	meta := &Meta[assocChild]{
		Table:      "kids",
		Columns:    []ColumnMeta{{Name: "id"}},
		SoftDelete: -1,
		Value:      func(c *assocChild, _ int) any { return c.ID },
	}
	r := Relation[assocParent, assocChild]{
		Name: "Kids", Kind: kind,
		Parent: TableRef{Name: "parents"}, Child: TableRef{Name: "kids"},
		ChildMeta:  meta,
		ParentCols: []string{"id"}, ChildCols: []string{"parent_id"}, ChildPK: []string{"id"},
		ParentKey: func(p *assocParent) ([]any, bool) { return []any{p.ID}, true },
		ChildKey:  func(c *assocChild) ([]any, bool) { return []any{c.ID}, true },
	}
	if kind == ManyToMany {
		r.ChildCols = []string{"id"}
		r.Through = &Through{Table: TableRef{Name: "jt"}, ParentCols: []string{"p"}, ChildCols: []string{"c"}}
	}
	return r
}

func TestAssociationChunkSizeAndRunChunks(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 4
	q := limitedQuerier{d: &d}
	r := assocRelation(HasMany)
	if got := r.chunkSize(q, 1, 1); got != 3 { // one fk bind leaves room for 3 keys
		t.Fatalf("chunkSize(1,1) = %d, want 3", got)
	}
	if got := r.chunkSize(q, 2, 0); got != 2 {
		t.Fatalf("chunkSize(2,0) = %d, want 2", got)
	}
	if got := r.chunkSize(q, 1, 9); got != 1 { // never below one key
		t.Fatalf("chunkSize(1,9) = %d, want 1", got)
	}
	keys := make([][]any, 7)
	for i := range keys {
		keys[i] = []any{int64(i)}
	}
	var sizes []int
	err := r.runChunks(context.Background(), q, keys, 3, func(_ Querier, ks [][]any) error {
		sizes = append(sizes, len(ks))
		return nil
	})
	if err != nil || len(sizes) != 3 || sizes[0] != 3 || sizes[1] != 3 || sizes[2] != 1 {
		t.Fatalf("chunks %v err=%v, want [3 3 1]", sizes, err)
	}
	boom := errors.New("boom")
	calls := 0
	err = r.runChunks(context.Background(), q, keys, 3, func(Querier, [][]any) error {
		calls++
		return boom
	})
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("a failing chunk must stop the run: calls=%d err=%v", calls, err)
	}
	if err := r.runChunks(context.Background(), q, nil, 3, func(Querier, [][]any) error {
		t.Fatal("no keys, no chunk")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
