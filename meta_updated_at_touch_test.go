// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

var touchDialects = []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL}

// touchQuote and touchParam give the identifier quote and first placeholder of
// a dialect, so one expectation covers all three.
func touchQuote(d dialect.Name) string {
	if d == dialect.MySQL {
		return "`"
	}
	return `"`
}

func touchParam(d dialect.Name) string {
	if d == dialect.Postgres {
		return "$1"
	}
	return "?"
}

// Catches: an Update with no Set failing with "no columns to update" on a
// model that has an updated_at column, instead of being a touch that sets only
// updated_at.
func TestUpdateWithoutSetTouchesUpdatedAt(t *testing.T) {
	for _, d := range touchDialects {
		db := momo.New(fakedriver.New(d))
		c, err := momo.Update[suite.Document](db).Where(suite.DocID.Eq(1)).SQL()
		if err != nil {
			t.Fatalf("%s: touch refused: %v", d, err)
		}
		q := touchQuote(d)
		head, _, _ := strings.Cut(c.SQL, " WHERE ")
		want := "UPDATE " + q + "documents" + q + " SET " + q + "updated_at" + q + " = " + touchParam(d)
		if head != want {
			t.Errorf("%s: SQL head = %s\nwant %s", d, head, want)
		}
		if _, ok := c.Args[0].(time.Time); !ok {
			t.Errorf("%s: first arg = %#v, want the update timestamp", d, c.Args[0])
		}
	}
}

// Catches: a touch bypassing the unfiltered-mutation guard, so an update with
// no Set and no Where rewrites updated_at on every row.
func TestUpdateTouchKeepsUnfilteredGuard(t *testing.T) {
	for _, d := range touchDialects {
		db := momo.New(fakedriver.New(d))
		if _, err := momo.Update[suite.Document](db).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("%s: touch without Where err = %v, want ErrUnfilteredMutation", d, err)
		}
		if _, err := momo.Update[suite.Document](db).AllRows().SQL(); err != nil {
			t.Errorf("%s: confirmed touch refused: %v", d, err)
		}
	}
}

// Catches: the touch also being allowed on a model that has no updated_at
// column, where there is nothing to set and the update must stay refused.
func TestUpdateWithoutSetStillRefusedWithoutUpdatedAt(t *testing.T) {
	id := momo.NewColumn[int64]("upd_only", "id", momo.FlagPK)
	for _, d := range touchDialects {
		db := momo.New(fakedriver.New(d))
		if _, err := momo.Update[updOnly](db).Where(id.Eq(1)).SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation (no columns to update)", d, err)
		}
	}
}

// Catches: a DoNothing conflict bumping updated_at, which would touch a row
// the statement is meant to leave alone.
func TestDoNothingDoesNotBumpUpdatedAt(t *testing.T) {
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t"}
	for _, d := range touchDialects {
		db := momo.New(fakedriver.New(d))
		c, err := momo.Insert[suite.Document](db).Model(&doc).OnConflict(suite.DocID).DoNothing().SQL()
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		marker := " ON CONFLICT"
		if d == dialect.MySQL {
			marker = " ON DUPLICATE KEY UPDATE "
		}
		_, tail, ok := strings.Cut(c.SQL, marker)
		if !ok || strings.Contains(tail, "updated_at") {
			t.Errorf("%s: DoNothing conflict clause bumps updated_at: %s", d, c.SQL)
		}
	}
}

// Catches: DoUpdate accepting an explicit assignment to updated_at (on Upsert
// or on a plain Insert) while Update refuses it, or the two paths giving
// different errors.
func TestDoUpdateRefusesExplicitUpdatedAt(t *testing.T) {
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t"}
	at := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	const phrase = `column "updated_at" is the updated-at column and cannot be set directly`
	for _, d := range touchDialects {
		db := momo.New(fakedriver.New(d))
		_, upd := momo.Update[suite.Document](db).Set(suite.DocUpdatedAt.Set(at)).Where(suite.DocID.Eq(1)).SQL()
		if upd == nil || !strings.Contains(upd.Error(), phrase) {
			t.Fatalf("%s: Update err = %v, want %q", d, upd, phrase)
		}
		builds := map[string]momo.InsertQuery[suite.Document]{
			"upsert":         momo.Upsert[suite.Document](db).Model(&doc).OnConflict(suite.DocID).DoUpdate(suite.DocUpdatedAt.Set(at)),
			"insert":         momo.Insert[suite.Document](db).Model(&doc).OnConflict(suite.DocID).DoUpdate(suite.DocUpdatedAt.Set(at)),
			"upsert-exclude": momo.Upsert[suite.Document](db).Model(&doc).OnConflict(suite.DocID).DoUpdate(suite.DocUpdatedAt.SetExcluded()),
			"insert-exclude": momo.Insert[suite.Document](db).Model(&doc).OnConflict(suite.DocID).DoUpdate(suite.DocUpdatedAt.SetExcluded()),
		}
		for name, qy := range builds {
			_, err := qy.SQL()
			if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), phrase) {
				t.Errorf("%s %s: err = %v, want ErrValidation containing %q", d, name, err, phrase)
			}
		}
	}
}
