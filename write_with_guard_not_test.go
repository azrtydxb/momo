// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: the second-source guard treating Not as a leaf, so that
// NOT (t.id = 1 AND s.title = 'x') (true for every target row whose source row
// has another title) passes as a filter on the target, and a double negation
// not returning to the original rule. Under negation an And needs every term on
// the target and an Or needs any term (De Morgan).
func TestFromUsingGuardNot(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.MySQL} {
		del := momo.Delete[delNote](withDB(d)).Using(withSrc)
		tgt, src := delNoteID.Eq(1), withSrcTitle.Eq("x")
		cases := []struct {
			name string
			p    momo.Predicate
			ok   bool
		}{
			{"Not(And(t,s))", momo.Not(momo.And(tgt, src)), false},
			{"Not(Or(t,s))", momo.Not(momo.Or(tgt, src)), true},
			{"Not(Not(Or(t,s)))", momo.Not(momo.Not(momo.Or(tgt, src))), false},
			{"Not(Not(And(t,s)))", momo.Not(momo.Not(momo.And(tgt, src))), true},
			{"Not(And(t,t))", momo.Not(momo.And(tgt, delNoteID.Eq(2))), true},
			{"Not(s)", momo.Not(src), false},
			{"Not(t)", momo.Not(tgt), true},
		}
		for _, c := range cases {
			_, err := del.Where(c.p).SQL()
			if c.ok && err != nil {
				t.Errorf("%s using %s: refused: %v", d, c.name, err)
			}
			if !c.ok && !errors.Is(err, momo.ErrUnfilteredMutation) {
				t.Errorf("%s using %s: err = %v, want ErrUnfilteredMutation", d, c.name, err)
			}
		}
	}
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		upd := momo.Update[unitDocument](withDB(d)).Set(DocTitle.SetExpr(withSrcTitle)).From(withSrc)
		if _, err := upd.Where(momo.Not(momo.And(DocID.Eq(1), withSrcID.Eq(2)))).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s from, Not(And(t,s)): err = %v, want ErrUnfilteredMutation", d, err)
		}
		if _, err := upd.Where(momo.Not(momo.Or(DocID.Eq(1), withSrcID.Eq(2)))).SQL(); err != nil {
			t.Errorf("%s from, Not(Or(t,s)): %v", d, err)
		}
	}
}

// Catches: JoinSQL with AND or OR treated as a leaf, so that
// JoinSQL("OR", t.id = 1, s.title = 'x') passes the guard like an Or with one
// branch off the target; a separator that is not AND or OR counting as a
// filter.
func TestFromUsingGuardJoinSQL(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.MySQL} {
		del := momo.Delete[delNote](withDB(d)).Using(withSrc)
		tgt, src := delNoteID.Eq(1), withSrcTitle.Eq("x")
		cases := []struct {
			name string
			p    momo.Predicate
			ok   bool
		}{
			{`JoinSQL("OR", t, s)`, momo.JoinSQL("OR", tgt, src), false},
			{`JoinSQL(" or ", t, s)`, momo.JoinSQL(" or ", tgt, src), false},
			{`JoinSQL("OR", t, t)`, momo.JoinSQL("OR", tgt, delNoteID.Eq(2)), true},
			{`JoinSQL("AND", t, s)`, momo.JoinSQL("AND", tgt, src), true},
			{`JoinSQL("AND", s, s)`, momo.JoinSQL("AND", src, withSrcTitle.Eq("y")), false},
			{`JoinSQL("OR")`, momo.JoinSQL("OR"), false},
			{`Not(JoinSQL("AND", t, s))`, momo.Not(momo.JoinSQL("AND", tgt, src)), false},
			{`Not(JoinSQL("OR", t, s))`, momo.Not(momo.JoinSQL("OR", tgt, src)), true},
			{`And(JoinSQL("OR", t, s), t)`, momo.And(momo.JoinSQL("OR", tgt, src), delNoteID.Eq(3)), true},
			{`JoinSQL("+", t, t)`, momo.JoinSQL("+", tgt, delNoteID.Eq(2)), false},
		}
		for _, c := range cases {
			_, err := del.Where(c.p).SQL()
			if c.ok && err != nil {
				t.Errorf("%s using %s: refused: %v", d, c.name, err)
			}
			if !c.ok && !errors.Is(err, momo.ErrUnfilteredMutation) {
				t.Errorf("%s using %s: err = %v, want ErrUnfilteredMutation", d, c.name, err)
			}
		}
	}
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		upd := momo.Update[unitDocument](withDB(d)).Set(DocTitle.SetExpr(withSrcTitle)).From(withSrc)
		if _, err := upd.Where(momo.JoinSQL("OR", DocID.Eq(1), withSrcID.Eq(2))).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf(`%s from, JoinSQL("OR", t, s): err = %v, want ErrUnfilteredMutation`, d, err)
		}
	}
}
