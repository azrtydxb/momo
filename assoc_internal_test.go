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

func TestAssociationRejectsStatementOverBindLimit(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 3
	q := limitedQuerier{d: &d}
	for _, kind := range []RelationKind{HasMany, ManyToMany} {
		r := assocRelation(kind)
		cs := []*assocChild{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
		for name, err := range map[string]error{
			"link":   r.Link(context.Background(), q, &assocParent{ID: 1}, cs...),
			"unlink": r.Unlink(context.Background(), q, &assocParent{ID: 1}, cs...),
		} {
			if !errors.Is(err, ErrValidation) {
				t.Errorf("%s %s over the bind limit: %v, want ErrValidation", kind, name, err)
			}
		}
	}
}
