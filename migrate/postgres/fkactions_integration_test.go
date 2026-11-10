// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fkcase"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

type pgRunner struct {
	t  *testing.T
	db *momo.DB
}

func (r pgRunner) Exec(q string) error {
	_, err := r.db.Driver().Exec(context.Background(), q, nil)
	return err
}

func (r pgRunner) Scalar(q string) string {
	r.t.Helper()
	rows, err := r.db.Driver().Query(context.Background(), q, nil)
	if err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		r.t.Fatalf("%s: no row", q)
	}
	var s string
	if err := rows.Scan(&s); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
	return s
}

func (pgRunner) TextType() string { return "text" }

func fkSchema(t *testing.T, cases []fkcase.Case, ext []schema.Extension) *schema.Schema {
	t.Helper()
	s, err := fkcase.Schema(dialect.Postgres, cases)
	if err != nil {
		t.Fatal(err)
	}
	s.Extensions = ext // database-wide; the test database installs vector
	return s
}

// TestFKActions: the models' foreign key tags become constraints through the
// real schema builder and diff, every referential action survives render,
// apply and inspection, does what it says on live rows, and a relation with
// the no_constraint tag leaves no foreign key to inspect.
//
// Catches: an action dropped by the renderer or the inspector (a CASCADE that
// round-trips as NO ACTION leaves the child row behind), a no_constraint that
// still creates a key, and NO ACTION rendered as RESTRICT.
func TestFKActions(t *testing.T) {
	db := testopen.Postgres(t)
	cases := fkcase.Cases(true)
	desired := fkSchema(t, cases, nil)
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{}))

	got := inspectNow(t, db)
	desired.Extensions = got.Extensions
	// The diff, not schema.Equal: the model spells NO ACTION out and the database
	// reports it as no clause, which the diff treats as the same.
	if again := planStatements(t, got, desired, migrate.DiffOptions{}); len(again) != 0 {
		t.Fatalf("the inspection differs from the models: %+v", again)
	}
	for _, tb := range got.Tables {
		if want := fkcase.WantForeignKeys(tb.Name); len(tb.ForeignKeys) != want {
			t.Errorf("table %s has %d foreign keys in the inspection, want %d", tb.Name, len(tb.ForeignKeys), want)
		}
	}
	if tb, ok := got.Table(fkcase.Loose); !ok || len(tb.ForeignKeys) != 0 {
		t.Fatalf("the no_constraint table must exist without a foreign key: %+v", tb)
	}
	fkcase.Check(t, pgRunner{t, db}, cases)
}

// NO ACTION and RESTRICT differ only when the constraint is deferred: NO ACTION
// is checked at the end of the transaction, RESTRICT at once. So a parent that
// is deleted and put back inside one transaction passes for NO ACTION and is
// refused for RESTRICT. This tells the two apart, which a plain delete cannot.
func TestFKActionsNoActionIsNotRestrict(t *testing.T) {
	db := testopen.Postgres(t)
	cases := []fkcase.Case{{Name: "del_noaction", OnDelete: "no_action"}, {Name: "del_restrict", OnDelete: "restrict"}, {Name: "del_default"}}
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, fkSchema(t, cases, nil), migrate.DiffOptions{}))
	for _, c := range cases {
		mustExec(t, db.Driver(), fmt.Sprintf(`ALTER TABLE fk_%s ALTER CONSTRAINT fk_%s_parent_id_fkey DEFERRABLE INITIALLY DEFERRED`, c.Name, c.Name))
	}
	ctx := context.Background()
	for _, c := range cases {
		pc := pinned(t, db)
		run := func(q string) error { _, err := pc.Exec(ctx, q, nil); return err }
		if err := run("BEGIN"); err != nil {
			t.Fatal(err)
		}
		must := func(q string) {
			t.Helper()
			if err := run(q); err != nil {
				t.Fatalf("%s: %s: %v", c.Name, q, err)
			}
		}
		must(`INSERT INTO fk_parents (id) VALUES (500)`)
		must(fmt.Sprintf(`INSERT INTO fk_%s (id, parent_id) VALUES (2, 500)`, c.Name))
		delErr := run(`DELETE FROM fk_parents WHERE id = 500`)
		if c.OnDelete == "restrict" {
			if delErr == nil {
				t.Errorf("%s: RESTRICT must refuse the delete at once even when deferred", c.Name)
			}
			_ = run("ROLLBACK")
			continue
		}
		if delErr != nil {
			t.Errorf("%s: NO ACTION is checked at commit, the delete must pass inside the transaction: %v", c.Name, delErr)
			_ = run("ROLLBACK")
			continue
		}
		must(`INSERT INTO fk_parents (id) VALUES (500)`)
		if err := run("COMMIT"); err != nil {
			t.Errorf("%s: parent put back before commit, the commit must pass: %v", c.Name, err)
		}
	}
}

// Changing an action on an existing table is a drop and an add of the key; the
// new action is the one the database enforces afterwards.
func TestFKActionsChangeOnExistingTable(t *testing.T) {
	db := testopen.Postgres(t)
	cases := fkcase.Cases(true)
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, fkSchema(t, cases, nil), migrate.DiffOptions{}))
	before := inspectNow(t, db)

	changed := slices.Clone(cases)
	for i := range changed {
		if changed[i].Name == "del_cascade" {
			changed[i].OnDelete = "set_null"
		}
	}
	v2 := fkSchema(t, changed, before.Extensions)
	stmts := planStatements(t, before, v2, migrate.DiffOptions{})
	var sql []string
	for _, s := range stmts {
		sql = append(sql, s.SQL)
	}
	if len(stmts) == 0 || !strings.Contains(strings.Join(sql, ";\n"), "ON DELETE SET NULL") {
		t.Fatalf("changing the action must render the new key:\n%s", strings.Join(sql, ";\n"))
	}
	applyStatements(t, db, stmts)
	if again := planStatements(t, inspectNow(t, db), v2, migrate.DiffOptions{}); len(again) != 0 {
		t.Fatalf("after the change the diff must be empty: %+v", again)
	}
	r := pgRunner{t, db}
	mustExec(t, db.Driver(), `INSERT INTO fk_parents (id) VALUES (600)`)
	mustExec(t, db.Driver(), `INSERT INTO fk_del_cascade (id, parent_id) VALUES (7, 600)`)
	mustExec(t, db.Driver(), `DELETE FROM fk_parents WHERE id = 600`)
	if got := r.Scalar(`SELECT COALESCE(parent_id::text, 'NULL') FROM fk_del_cascade WHERE id = 7`); got != "NULL" {
		t.Fatalf("the changed key must SET NULL, the child has parent_id %s", got)
	}
}
