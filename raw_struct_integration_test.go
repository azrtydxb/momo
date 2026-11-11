// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

type wideTail struct {
	Name string
	Note *string
	Rank int64 `momo:"column:rank_no"`
}

// wideRow reads 13 columns with no generated metadata.
type wideRow struct {
	ID       int64
	Tenant   int64
	UserID   int64
	Qty      int64
	Price    int64
	Version  int64
	Flag     int64
	Weight   int64
	Level    int64
	Position int64
	wideTail
}

// Catches: a plain struct of 13 columns mis-scanned on a real driver
// (PostgreSQL, MySQL, MariaDB or SQLite): by-name matching in a different
// order than the struct, a NULL into a pointer field, the embedded struct.
func TestQueryPlainStructThirteenColumns(t *testing.T) {
	rqEach(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, s := range []string{
			"CREATE TABLE rs_wide (id INTEGER PRIMARY KEY, tenant INTEGER NOT NULL, user_id INTEGER NOT NULL, qty INTEGER NOT NULL, price INTEGER NOT NULL, version INTEGER NOT NULL, flag INTEGER NOT NULL, weight INTEGER NOT NULL, level INTEGER NOT NULL, position INTEGER NOT NULL, name VARCHAR(40) NOT NULL, note VARCHAR(40), rank_no INTEGER NOT NULL)",
			"INSERT INTO rs_wide VALUES (1, 10, 20, 3, 4, 5, 6, 7, 8, 9, 'one', 'n1', 100), (2, 11, 21, 4, 5, 6, 7, 8, 9, 10, 'two', NULL, 200)",
		} {
			if _, err := momo.Exec(ctx, db, s); err != nil {
				t.Fatal(err)
			}
		}
		const stmt = "SELECT rank_no, note, name, position, level, weight, flag, version, price, qty, user_id, tenant, id FROM rs_wide WHERE tenant >= ? ORDER BY id"
		got, err := momo.Query[wideRow](ctx, db, stmt, 10)
		if err != nil || len(got) != 2 {
			t.Fatalf("Query = %+v, %v", got, err)
		}
		a, b := got[0], got[1]
		if a.ID != 1 || a.Tenant != 10 || a.UserID != 20 || a.Position != 9 || a.Name != "one" || a.Rank != 100 || a.Note == nil || *a.Note != "n1" {
			t.Errorf("row 1 = %+v", a)
		}
		if b.ID != 2 || b.Name != "two" || b.Note != nil || b.Rank != 200 {
			t.Errorf("row 2 = %+v", b)
		}
		n := 0
		for r, err := range momo.QueryRows[wideRow](ctx, db, stmt, 10) {
			if err != nil {
				t.Fatal(err)
			}
			n += int(r.ID)
		}
		if n != 3 {
			t.Errorf("QueryRows ids sum = %d", n)
		}
		_, err = momo.Query[wideRow](ctx, db, "SELECT id, surprise_col FROM (SELECT id, 1 AS surprise_col FROM rs_wide) s")
		if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "surprise_col") {
			t.Errorf("unmatched column: err = %v, want ErrValidation naming surprise_col", err)
		}
	})
}
