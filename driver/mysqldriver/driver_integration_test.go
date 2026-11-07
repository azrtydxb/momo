// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysqldriver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// servers lists the opener of every supported server.
var servers = []struct {
	name string
	open func(*testing.T, ...momo.Option) *momo.DB
}{
	{"mysql84", testopen.MySQL84},
	{"mysql97", testopen.MySQL97},
	{"mariadb118", testopen.MariaDB},
}

// mustExecMySQL fails the test on any error from momo.Exec.
func mustExecMySQL(t *testing.T, q momo.Querier, sql string, args ...any) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), q, sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestErrorClassification(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE users (id bigint PRIMARY KEY AUTO_INCREMENT, email varchar(255) UNIQUE)")
			mustExecMySQL(t, db, "CREATE TABLE orders (id bigint PRIMARY KEY AUTO_INCREMENT, user_id bigint NOT NULL, FOREIGN KEY (user_id) REFERENCES users(id))")
			mustExecMySQL(t, db, "CREATE TABLE checked (x integer, CONSTRAINT positive CHECK (x > 0))")
			mustExecMySQL(t, db, "INSERT INTO users (email) VALUES ('a@example.com')")
			mustExecMySQL(t, db, "INSERT INTO orders (user_id) VALUES (1)")
			cases := []struct {
				name string
				sql  string
				kind error
			}{
				{"unique (1062)", "INSERT INTO users (email) VALUES ('a@example.com')", momo.ErrUniqueViolation},
				{"foreign key insert (1452)", "INSERT INTO orders (user_id) VALUES (999)", momo.ErrForeignKeyViolation},
				{"foreign key delete (1451)", "DELETE FROM users WHERE id = 1", momo.ErrForeignKeyViolation},
				{"not null (1048)", "INSERT INTO orders (user_id) VALUES (NULL)", momo.ErrNotNullViolation},
				{"check", "INSERT INTO checked (x) VALUES (0)", momo.ErrCheckViolation},
			}
			for _, c := range cases {
				_, err := momo.Exec(context.Background(), db, c.sql)
				if !errors.Is(err, c.kind) {
					t.Errorf("%s: err = %v, want %v", c.name, err, c.kind)
				}
			}
		})
	}
}

type emptyRow struct{ ID int64 }

func (r *emptyRow) MomoMeta() *momo.Meta[emptyRow] {
	return &momo.Meta[emptyRow]{
		Table:   "empties",
		PK:      []int{0},
		Columns: []momo.ColumnMeta{{Name: "id"}},
		Ptr:     func(r *emptyRow, i int) any { return &r.ID },
		Value:   func(r *emptyRow, i int) any { return r.ID },
	}
}

func TestEmptyResultNotNotFound(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE empties (id bigint PRIMARY KEY)")
			rows, err := momo.Select[emptyRow, *emptyRow](db).All(context.Background())
			if err != nil {
				t.Fatalf("All on zero rows: err = %v, want nil", err)
			}
			if len(rows) != 0 {
				t.Fatalf("All on zero rows returned %d rows", len(rows))
			}
		})
	}
}

func TestErrorRedaction(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE accounts (id bigint PRIMARY KEY AUTO_INCREMENT, email varchar(255) UNIQUE)")
			mustExecMySQL(t, db, "INSERT INTO accounts (email) VALUES ('secret@example.com')")
			_, err := momo.Exec(context.Background(), db, "INSERT INTO accounts (email) VALUES ('secret@example.com')")
			var me *momo.Error
			if !errors.As(err, &me) {
				t.Fatalf("err = %v, want a *momo.Error", err)
			}
			if strings.Contains(err.Error(), "secret@example.com") {
				t.Fatal("Error() leaked the offending value")
			}
			if !strings.Contains(me.Detail(), "secret@example.com") {
				t.Fatal("Detail() lost the driver message")
			}
		})
	}
}

// TestRowsAffectedCountsMatched proves clientFoundRows: an UPDATE that sets a
// column to its current value still reports the matched row.
func TestRowsAffectedCountsMatched(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE ra (id bigint PRIMARY KEY, v int)")
			mustExecMySQL(t, db, "INSERT INTO ra VALUES (1, 5)")
			n, err := db.Driver().Exec(context.Background(), "UPDATE ra SET v = 5 WHERE id = 1", nil)
			if err != nil || n != 1 {
				t.Fatalf("rows affected = %d, err = %v, want 1", n, err)
			}
		})
	}
}

func TestPinAndTransaction(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			ctx := context.Background()
			db := s.open(t)
			mustExecMySQL(t, db, "CREATE TABLE tx_t (id bigint PRIMARY KEY)")
			err := db.Tx(ctx, func(tx *momo.Tx) error {
				_, err := momo.Exec(ctx, tx, "INSERT INTO tx_t VALUES (1)")
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			pc, err := db.Driver().(momo.Pinner).Pin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pc.Exec(ctx, "SELECT GET_LOCK('momo_t', 1)", nil); err != nil {
				t.Fatal(err)
			}
			if err := pc.(momo.Discarder).Discard(); err != nil {
				t.Fatal(err)
			}
			if err := pc.Release(); err != nil {
				t.Fatalf("Release after Discard = %v, want nil", err)
			}
		})
	}
}
