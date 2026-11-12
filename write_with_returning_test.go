// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: a data-modifying CTE body that nothing reads being refused for want
// of Returning. WITH d AS (DELETE ...) INSERT ... is valid PostgreSQL, so the
// build must let it through; a body needs Returning only when it is read.
func TestUnreadDMLCTEBodyWithoutReturningBuilds(t *testing.T) {
	pg := withDB(dialect.Postgres)
	body := momo.Delete[delNote](pg).Where(delNoteID.Eq(2))
	c, err := momo.Insert[delNote](pg).With("d", body).Values(delNoteID.Set(9), delNoteTitle.Set("n")).SQL()
	if err != nil || !strings.HasPrefix(c.SQL, `WITH "d" AS (DELETE FROM "del_notes" WHERE "del_notes"."id" = $1) INSERT INTO`) {
		t.Errorf("insert after an unread DELETE body: %q %v", c.SQL, err)
	}
	upd := momo.Update[delNote](pg).Set(delNoteTitle.Set("u")).Where(delNoteID.Eq(1))
	c, err = momo.Delete[delNote](pg).With("u", upd).Where(delNoteID.Eq(3)).SQL()
	if err != nil || !strings.HasPrefix(c.SQL, `WITH "u" AS (UPDATE "del_notes" SET "title" = $1 WHERE "del_notes"."id" = $2) DELETE FROM`) {
		t.Errorf("delete after an unread UPDATE body: %q %v", c.SQL, err)
	}
}
