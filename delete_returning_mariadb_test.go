// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func flavorDB(f dialect.Flavor) *momo.DB {
	return momo.New(fakedriver.NewDialect(dialect.MySQLFlavor(f)))
}

// Catches: DELETE ... RETURNING refused on MariaDB, or rendered with the wrong
// column list or quoting.
func TestDeleteReturningRendersOnMariaDB(t *testing.T) {
	ma := flavorDB(dialect.FlavorMariaDB)
	c, err := momo.Delete[delNote](ma).Where(delNoteID.Eq(1)).Returning(delNoteTitle, delNoteID).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := "DELETE FROM `del_notes` WHERE `del_notes`.`id` = ? RETURNING `title`, `id`"; c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
	c, err = momo.Delete[delNote](ma).Where(delNoteID.Eq(1)).ReturningAll().SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := "DELETE FROM `del_notes` WHERE `del_notes`.`id` = ? RETURNING `id`, `title`"; c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
}

// Catches: MySQL accepting DELETE ... RETURNING, or its error not pointing at
// MariaDB.
func TestDeleteReturningRefusedOnMySQLFlavor(t *testing.T) {
	my := flavorDB(dialect.FlavorMySQL)
	_, err := momo.Delete[delNote](my).Where(delNoteID.Eq(1)).Returning(delNoteID).SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if !strings.Contains(err.Error(), "MariaDB") {
		t.Fatalf("err = %v, want it to name MariaDB", err)
	}
}

// Catches: RETURNING sent with a multi-table DELETE, which MariaDB rejects at
// the server, or the refusal leaking to a plain MariaDB delete.
func TestDeleteUsingReturningRefusedOnMariaDB(t *testing.T) {
	ma := flavorDB(dialect.FlavorMariaDB)
	_, err := momo.Delete[delNote](ma).Using(withSrc).Where(delNoteID.EqExpr(withSrcID)).Returning(delNoteID).SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// Catches: UPDATE ... RETURNING enabled on MariaDB by the delete change.
func TestUpdateReturningStillRefusedOnMariaDB(t *testing.T) {
	ma := flavorDB(dialect.FlavorMariaDB)
	_, err := momo.Update[delNote](ma).Set(delNoteTitle.Set("x")).Where(delNoteID.Eq(1)).Returning(delNoteID).SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}
