// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

// Errors asserts the driver-independent error classification S-8 requires:
// unique, not-null, check and foreign-key violations map to their sentinels,
// carry the table, and keep the driver cause reachable through Unwrap.
func Errors(t *testing.T, db *momo.DB, caps Caps) {
	ctx := context.Background()
	CreateDocuments(t, db, caps)
	for _, stmt := range []string{
		"CREATE UNIQUE INDEX documents_title_uq ON documents (title)",
		"DROP TABLE IF EXISTS checked",
		"CREATE TABLE checked (x integer check (x > 0))",
		"DROP TABLE IF EXISTS child",
		"DROP TABLE IF EXISTS parent",
		"CREATE TABLE parent (id integer primary key)",
		"CREATE TABLE child (id integer primary key, parent_id integer not null references parent(id))",
	} {
		if _, err := momo.Exec(ctx, db, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	t.Cleanup(func() {
		for _, tbl := range []string{"child", "parent", "checked"} {
			if _, err := momo.Exec(ctx, db, "DROP TABLE IF EXISTS "+tbl); err != nil {
				t.Errorf("drop %s: %v", tbl, err)
			}
		}
	})

	check := func(t *testing.T, err error, want error) *momo.Error {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
		var me *momo.Error
		if !errors.As(err, &me) {
			t.Fatalf("err %v is not a *momo.Error", err)
		}
		if errors.Unwrap(err) == nil {
			t.Errorf("classified error %v has no driver cause", err)
		}
		return me
	}

	t.Run("unique", func(t *testing.T) {
		if err := insertDoc(ctx, db, "dup"); err != nil {
			t.Fatal(err)
		}
		me := check(t, insertDoc(ctx, db, "dup"), momo.ErrUniqueViolation)
		if me.Table != "documents" {
			t.Errorf("Table = %q, want documents", me.Table)
		}
	})

	t.Run("not null", func(t *testing.T) {
		_, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, NULL)")
		check(t, err, momo.ErrNotNullViolation)
	})

	t.Run("check", func(t *testing.T) {
		_, err := momo.Exec(ctx, db, "INSERT INTO checked (x) VALUES (?)", 0)
		check(t, err, momo.ErrCheckViolation)
	})

	t.Run("foreign key", func(t *testing.T) {
		_, err := momo.Exec(ctx, db, "INSERT INTO child (id, parent_id) VALUES (1, 999)")
		check(t, err, momo.ErrForeignKeyViolation)
	})
}
