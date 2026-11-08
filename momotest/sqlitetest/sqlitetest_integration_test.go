// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitetest_test

import (
	"os"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/momotest"
	"github.com/azrtydxb/momo/momotest/sqlitetest"
)

// databaseFile returns the path of the main database of db.
func databaseFile(t *testing.T, db *momo.DB) string {
	t.Helper()
	rows, err := db.Driver().Query(t.Context(), `PRAGMA database_list`, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	for rows.Next() {
		var seq int64
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			t.Fatal(err)
		}
		if name == "main" {
			return file
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("database file path not found")
	return ""
}

// TestMomotestSQLiteEphemeral: every call opens a database of its own, and the
// file is gone when the test that opened it has ended.
func TestMomotestSQLiteEphemeral(t *testing.T) {
	ctx := t.Context()
	var file string
	t.Run("inner", func(t *testing.T) {
		db := sqlitetest.New(t)
		if _, err := momo.Exec(ctx, db, `CREATE TABLE only_here (x INTEGER)`); err != nil {
			t.Fatal(err)
		}
		file = databaseFile(t, db)
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("database file %s does not exist while the test runs: %v", file, err)
		}
		other := sqlitetest.New(t)
		if _, err := momo.Exec(ctx, other, `SELECT x FROM only_here`); err == nil {
			t.Fatal("second SQLite database sees the first one's table")
		}
		if databaseFile(t, other) == file {
			t.Fatal("two calls share one database file")
		}
	})
	if file == "" {
		t.Fatal("database file path not found")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("database file %s still exists after the test (err %v)", file, err)
	}
}

// TestSQLiteOptionsAndFixtures: options reach momo.New, sqlite-vec is loaded,
// and LoadFixtures fills the database.
func TestSQLiteOptionsAndFixtures(t *testing.T) {
	rec := &obstest.Recorder{}
	db := sqlitetest.New(t, momo.WithHook(rec))
	for _, ddl := range []string{
		`CREATE TABLE tags (id BIGINT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE authors (id BIGINT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE documents (id BIGINT PRIMARY KEY, author_id BIGINT REFERENCES authors(id), title TEXT NOT NULL)`,
	} {
		if _, err := momo.Exec(t.Context(), db, ddl); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.SQLs()) == 0 {
		t.Fatal("the hook given to sqlitetest.New saw no statement")
	}
	momotest.LoadFixtures(t, db, "../testdata/fixtures.yaml")
	titles, err := momo.Scalars[string](t.Context(), db, "SELECT title FROM documents ORDER BY id")
	if err != nil || len(titles) != 2 || titles[0] != "A1" || titles[1] != "Orphan" {
		t.Fatalf("titles = %v, %v", titles, err)
	}
	version, err := momo.Scalars[string](t.Context(), db, "SELECT vec_version()")
	if err != nil || len(version) != 1 || version[0] == "" {
		t.Fatalf("vec_version() = %v, %v, want sqlite-vec loaded", version, err)
	}
}
