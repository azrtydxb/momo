// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"testing"
)

func validRel() Relation[int, int] {
	return Relation[int, int]{
		Name: "Child", Kind: HasMany,
		Parent: TableRef{Name: "p"}, Child: TableRef{Name: "c"},
		ParentCols: []string{"id"}, ChildCols: []string{"pid"}, ChildPK: []string{"id"},
		ParentKey:   func(p *int) ([]any, bool) { return []any{*p}, true },
		ChildKey:    func(c *int) ([]any, bool) { return []any{*c}, true },
		SetChildKey: func(c *int, k []any) {},
		GetMany:     func(p *int) []int { return nil },
		SetMany:     func(p *int, cs []int) {},
	}
}

func TestRelationValidate(t *testing.T) {
	cases := map[string]struct {
		mut  func(r *Relation[int, int])
		want bool // true: valid
	}{
		"valid has-many":             {func(r *Relation[int, int]) {}, true},
		"no name":                    {func(r *Relation[int, int]) { r.Name = "" }, false},
		"arity mismatch":             {func(r *Relation[int, int]) { r.ChildCols = []string{"a", "b"} }, false},
		"missing ParentKey":          {func(r *Relation[int, int]) { r.ParentKey = nil }, false},
		"has-many without GetMany":   {func(r *Relation[int, int]) { r.GetMany = nil }, false},
		"to-one needs SetOne":        {func(r *Relation[int, int]) { r.Kind = BelongsTo; r.GetOne = func(*int) *int { return nil } }, false},
		"through on has-many":        {func(r *Relation[int, int]) { r.Through = &Through{ParentCols: []string{"x"}, ChildCols: []string{"y"}} }, false},
		"many-to-many needs through": {func(r *Relation[int, int]) { r.Kind = ManyToMany }, false},
		"discriminator on to-one": {func(r *Relation[int, int]) {
			r.Kind = BelongsTo
			r.GetOne = func(*int) *int { return nil }
			r.SetOne = func(*int, *int) {}
			r.Discriminator = &Discriminator{Column: "t", Value: "x"}
		}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := validRel()
			tc.mut(&r)
			err := r.validate()
			if tc.want && err != nil {
				t.Fatalf("want valid, got %v", err)
			}
			if !tc.want && !errors.Is(err, ErrValidation) {
				t.Fatalf("want ErrValidation, got %v", err)
			}
		})
	}
}
