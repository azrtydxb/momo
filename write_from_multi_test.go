// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

var (
	withOther    = momo.CTE("other")
	withOtherID  = momo.ColumnOf[int64]("other", "id")
	withOtherTag = momo.ColumnOf[string]("other", "tag")
)

// Catches: From with a second source rendering only the first or an
// unseparated list, in any dialect (PostgreSQL and SQLite FROM a, b; MySQL
// multi-table UPDATE t, a, b).
func TestUpdateFromTwoSourcesSQL(t *testing.T) {
	cases := []struct {
		d    dialect.Name
		want string
	}{
		{dialect.Postgres, `UPDATE "del_notes" SET "title" = "src"."title" FROM "src", "other" WHERE ("del_notes"."id" = "src"."id") AND ("del_notes"."id" = "other"."id")`},
		{dialect.SQLite, `UPDATE "del_notes" SET "title" = "src"."title" FROM "src", "other" WHERE ("del_notes"."id" = "src"."id") AND ("del_notes"."id" = "other"."id")`},
		{dialect.MySQL, "UPDATE `del_notes`, `src`, `other` SET `del_notes`.`title` = `src`.`title` WHERE (`del_notes`.`id` = `src`.`id`) AND (`del_notes`.`id` = `other`.`id`)"},
	}
	for _, tc := range cases {
		c, err := momo.Update[delNote](withDB(tc.d)).Set(delNoteTitle.SetExpr(withSrcTitle)).
			From(withSrc, withOther).
			Where(delNoteID.EqExpr(withSrcID), delNoteID.EqExpr(withOtherID)).SQL()
		if err != nil || c.SQL != tc.want {
			t.Errorf("update %s: got %q %v, want %q", tc.d, c.SQL, err, tc.want)
		}
	}
	for d, want := range map[dialect.Name]string{
		dialect.Postgres: `DELETE FROM "del_notes" USING "src", "other" WHERE ("del_notes"."id" = "src"."id") AND ("del_notes"."id" = "other"."id")`,
		dialect.MySQL:    "DELETE `del_notes` FROM `del_notes`, `src`, `other` WHERE (`del_notes`.`id` = `src`.`id`) AND (`del_notes`.`id` = `other`.`id`)",
	} {
		c, err := momo.Delete[delNote](withDB(d)).Using(withSrc, withOther).
			Where(delNoteID.EqExpr(withSrcID), delNoteID.EqExpr(withOtherID)).SQL()
		if err != nil || c.SQL != want {
			t.Errorf("delete %s: got %q %v, want %q", d, c.SQL, err, want)
		}
	}
}

// Catches: RETURNING columns of the FROM / USING source dropped, rendered
// unqualified (ambiguous when source and target share a name) or placed out of
// order; ReturningAll losing the target columns that come first.
func TestReturningSourceColumnsSQL(t *testing.T) {
	upd := momo.Update[delNote](withDB(dialect.Postgres)).Set(delNoteTitle.SetExpr(withSrcTitle)).
		From(withSrc, withOther).
		Where(delNoteID.EqExpr(withSrcID), delNoteID.EqExpr(withOtherID))
	for name, tc := range map[string]struct {
		q    momo.UpdateQuery[delNote]
		want string
	}{
		"list": {upd.Returning(delNoteID, withSrcTitle, withOtherTag), `"del_notes"."id", "src"."title", "other"."tag"`},
		"all":  {upd.ReturningAll().Returning(withOtherTag), `"del_notes"."id", "del_notes"."title", "other"."tag"`},
		"own":  {upd.Returning(delNoteTitle), `"del_notes"."title"`},
	} {
		c, err := tc.q.SQL()
		want := ` AND ("del_notes"."id" = "other"."id") RETURNING ` + tc.want
		if err != nil || len(c.SQL) < len(want) || c.SQL[len(c.SQL)-len(want):] != want {
			t.Errorf("update %s: got %q %v, want a tail %q", name, c.SQL, err, want)
		}
	}
	del := momo.Delete[delNote](withDB(dialect.Postgres)).Using(withSrc).Where(delNoteID.EqExpr(withSrcID)).Returning(delNoteID, withSrcTitle)
	c, err := del.SQL()
	if want := `DELETE FROM "del_notes" USING "src" WHERE "del_notes"."id" = "src"."id" RETURNING "del_notes"."id", "src"."title"`; err != nil || c.SQL != want {
		t.Errorf("delete: got %q %v, want %q", c.SQL, err, want)
	}
}

// Catches: a source column in RETURNING reaching SQLite (whose RETURNING reads
// the modified table only) or MySQL (no RETURNING), and Rows scanning a source
// column into the model, where it has no field.
func TestReturningSourceColumnsRefused(t *testing.T) {
	q := func(d dialect.Name) momo.UpdateQuery[delNote] {
		return momo.Update[delNote](withDB(d)).Set(delNoteTitle.SetExpr(withSrcTitle)).From(withSrc).
			Where(delNoteID.EqExpr(withSrcID)).Returning(delNoteID, withSrcTitle)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		if _, err := q(d).SQL(); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s err = %v, want ErrUnsupported", d, err)
		}
	}
	if _, err := q(dialect.Postgres).Rows(context.Background()); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("update Rows err = %v, want ErrValidation", err)
	}
	_, err := momo.Delete[delNote](withDB(dialect.Postgres)).Using(withSrc).Where(delNoteID.EqExpr(withSrcID)).
		Returning(withSrcTitle).Rows(context.Background())
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("delete Rows err = %v, want ErrValidation", err)
	}
	// A source column with no source at all names a table the statement lacks.
	_, err = momo.Update[delNote](withDB(dialect.Postgres)).Set(delNoteTitle.Set("x")).Where(delNoteID.Eq(1)).Returning(withSrcTitle).SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("source column without From err = %v, want ErrValidation", err)
	}
}

// Catches: the unfiltered-mutation guard checking only the target when there
// are several sources, so a source that no predicate references is silently
// cross-joined to the statement. Each source must be constrained.
func TestFromUsingGuardSeveralSources(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.MySQL} {
		del := momo.Delete[delNote](withDB(d)).Using(withSrc, withOther)
		if _, err := del.Where(delNoteID.EqExpr(withSrcID)).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s using, second source unconstrained: err = %v, want ErrUnfilteredMutation", d, err)
		}
		if _, err := del.Where(delNoteID.EqExpr(withOtherID)).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s using, first source unconstrained: err = %v, want ErrUnfilteredMutation", d, err)
		}
		if _, err := del.Where(momo.Or(delNoteID.EqExpr(withSrcID), delNoteID.EqExpr(withOtherID))).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s using, each source in one Or branch: err = %v, want ErrUnfilteredMutation", d, err)
		}
		if _, err := del.Where(delNoteID.EqExpr(withSrcID)).AllRows().SQL(); err != nil {
			t.Errorf("%s using, AllRows: %v", d, err)
		}
		if _, err := del.Where(delNoteID.EqExpr(withSrcID), withOtherID.EqExpr(withSrcID)).SQL(); err != nil {
			t.Errorf("%s using, both constrained: %v", d, err)
		}
	}
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		upd := momo.Update[delNote](withDB(d)).Set(delNoteTitle.SetExpr(withSrcTitle)).From(withSrc, withOther)
		if _, err := upd.Where(delNoteID.EqExpr(withSrcID)).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s from, second source unconstrained: err = %v, want ErrUnfilteredMutation", d, err)
		}
		if _, err := upd.Where(delNoteID.Eq(1)).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s from, no source constrained: err = %v, want ErrUnfilteredMutation", d, err)
		}
		if _, err := upd.Where(delNoteID.EqExpr(withSrcID)).AllRows().SQL(); err != nil {
			t.Errorf("%s from, AllRows: %v", d, err)
		}
		if _, err := upd.Where(delNoteID.EqExpr(withSrcID), delNoteID.EqExpr(withOtherID)).SQL(); err != nil {
			t.Errorf("%s from, both constrained: %v", d, err)
		}
		// One source stays checked against the target only.
		if _, err := momo.Update[delNote](withDB(d)).Set(delNoteTitle.SetExpr(withSrcTitle)).From(withSrc).Where(delNoteID.Eq(1)).SQL(); err != nil {
			t.Errorf("%s from, one source feeding SET only: %v", d, err)
		}
	}
}
