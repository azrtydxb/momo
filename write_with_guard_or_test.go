// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: the second-source guard accepting an Or when only one branch
// references the target (the other branch is true on its own, so the cross
// join touches every target row), refusing an Or whose branches all reference
// the target, or refusing an And that has one target term.
func TestFromUsingGuardOr(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.MySQL} {
		del := momo.Delete[delNote](withDB(d)).Using(withSrc)
		mixed := momo.Or(delNoteID.Eq(1), withSrcTitle.Eq("x"))
		if _, err := del.Where(mixed).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s using, Or with a branch off the target: err = %v", d, err)
		}
		if _, err := del.Where(mixed).AllRows().SQL(); err != nil {
			t.Errorf("%s using, mixed Or with AllRows: %v", d, err)
		}
		if _, err := del.Where(momo.Or(delNoteID.Eq(1), delNoteID.EqExpr(withSrcID))).SQL(); err != nil {
			t.Errorf("%s using, Or on the target only: %v", d, err)
		}
		if _, err := del.Where(momo.And(delNoteID.Eq(1), withSrcTitle.Eq("x"))).SQL(); err != nil {
			t.Errorf("%s using, And with one target term: %v", d, err)
		}
		if _, err := del.Where(momo.Or(momo.And(delNoteID.Eq(1), withSrcTitle.Eq("x")), delNoteID.Eq(2))).SQL(); err != nil {
			t.Errorf("%s using, Or of an And with a target term: %v", d, err)
		}
		if _, err := del.Where(momo.And(delNoteID.EqExpr(withSrcID), momo.Or(delNoteID.Eq(1), withSrcTitle.Eq("x")))).SQL(); err != nil {
			t.Errorf("%s using, target term beside a mixed Or: %v", d, err)
		}
	}
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		upd := momo.Update[unitDocument](withDB(d)).Set(DocTitle.SetExpr(withSrcTitle)).From(withSrc)
		mixed := momo.Or(DocID.Eq(1), withSrcID.Eq(2))
		if _, err := upd.Where(mixed).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s from, Or with a branch off the target: err = %v", d, err)
		}
		if _, err := upd.Where(mixed).AllRows().SQL(); err != nil {
			t.Errorf("%s from, mixed Or with AllRows: %v", d, err)
		}
		if _, err := upd.Where(momo.Or(DocID.Eq(1), DocID.EqExpr(withSrcID))).SQL(); err != nil {
			t.Errorf("%s from, Or on the target only: %v", d, err)
		}
		if _, err := upd.Where(momo.And(DocID.Eq(1), withSrcID.Eq(2))).SQL(); err != nil {
			t.Errorf("%s from, And with one target term: %v", d, err)
		}
	}
}
