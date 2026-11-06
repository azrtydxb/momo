// SPDX-License-Identifier: Apache-2.0

package momo

import "testing"

type finishParent struct{ Child *finishChild }
type finishChild struct{ N int }

// pending.finish copies the pooled child per parent: a generated to-one setter
// stores the pointer it is given (p.Author = c), so handing out &pool[j] would
// alias one value across every parent that shares the child. The copy escapes
// to the heap, so it is not a dangling stack pointer.
func TestFinishToOneParentsDoNotAliasSharedChild(t *testing.T) {
	r := &Relation[finishParent, finishChild]{
		Kind:   BelongsTo,
		SetOne: func(p *finishParent, c *finishChild) { p.Child = c },
	}
	pd := &pending[finishParent, finishChild]{
		r:       r,
		parents: make([]finishParent, 2),
		pool:    []finishChild{{N: 1}},
		owners:  [][]int{{0, 1}},
	}
	if err := pd.finish(t.Context()); err != nil {
		t.Fatal(err)
	}
	a, b := pd.parents[0].Child, pd.parents[1].Child
	if a == nil || b == nil || a.N != 1 || b.N != 1 {
		t.Fatalf("children not assigned: %+v %+v", a, b)
	}
	if a == b || a == &pd.pool[0] || b == &pd.pool[0] {
		t.Fatal("parents share a child pointer")
	}
	a.N = 9
	if b.N != 1 || pd.pool[0].N != 1 {
		t.Fatal("mutating one parent's child leaked into another")
	}
}
