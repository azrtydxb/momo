// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fkcase"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
)

type myRunner struct {
	t  *testing.T
	db *momo.DB
}

func (r myRunner) Exec(q string) error {
	_, err := r.db.Driver().Exec(context.Background(), q, nil)
	return err
}

func (r myRunner) Scalar(q string) string {
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

func (myRunner) TextType() string { return "CHAR" }

// TestFKActions: the models' foreign key tags become constraints through the
// real schema builder and diff, every referential action InnoDB accepts
// survives render, apply and inspection, does what it says on live rows, and a
// relation with the no_constraint tag leaves no foreign key to inspect. SET
// DEFAULT is not a case: InnoDB rejects it. InnoDB treats NO ACTION as RESTRICT,
// so the two are not told apart here (PostgreSQL's test does).
//
// Catches: an action dropped by the renderer or the inspector (a CASCADE that
// round-trips as NO ACTION leaves the child row behind), and a no_constraint
// that still creates a key.
func TestFKActions(t *testing.T) {
	ctx := context.Background()
	for _, tg := range targets {
		t.Run(tg.name, func(t *testing.T) {
			db := tg.open(t)
			dir := t.TempDir()
			eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
			cases := fkcase.Cases(false)
			desired, err := fkcase.Schema(dialect.MySQL, cases)
			if err != nil {
				t.Fatal(err)
			}
			applyExample(t, dir, eng, desired, "fk_actions")

			live, err := eng.Inspect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// The diff, not schema.Equal: the model spells NO ACTION out and the
			// server reports it as no clause, which the diff treats as the same.
			if changes, _, err := migrate.Diff(live, desired, migrate.DiffOptions{}); err != nil || len(changes) != 0 {
				t.Fatalf("the inspection differs from the models: %+v %v", changes, err)
			}
			if again, err := eng.Diff(ctx, desired, "noop", migrate.DiffOptions{}); err != nil || again != nil {
				t.Fatalf("a second Engine.Diff must write nothing: %v %v", again, err)
			}
			for _, tb := range live.Tables {
				if want := fkcase.WantForeignKeys(tb.Name); len(tb.ForeignKeys) != want {
					t.Errorf("table %s has %d foreign keys in the inspection, want %d", tb.Name, len(tb.ForeignKeys), want)
				}
			}
			if tb, ok := live.Table(fkcase.Loose); !ok || len(tb.ForeignKeys) != 0 {
				t.Fatalf("the no_constraint table must exist without a foreign key: %+v", tb)
			}
			fkcase.Check(t, myRunner{t, db}, cases)
		})
	}
}

// Changing an action on an existing table drops and re-adds the key; the new
// action is the one the server enforces afterwards.
func TestFKActionsChangeOnExistingTable(t *testing.T) {
	for _, tg := range targets {
		t.Run(tg.name, func(t *testing.T) {
			db := tg.open(t)
			dir := t.TempDir()
			eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
			cases := fkcase.Cases(false)
			v1, err := fkcase.Schema(dialect.MySQL, cases)
			if err != nil {
				t.Fatal(err)
			}
			applyExample(t, dir, eng, v1, "fk_v1")

			changed := slices.Clone(cases)
			for i := range changed {
				if changed[i].Name == "del_cascade" {
					changed[i].OnDelete = "set_null"
				}
			}
			v2, err := fkcase.Schema(dialect.MySQL, changed)
			if err != nil {
				t.Fatal(err)
			}
			applyExample(t, dir, eng, v2, "fk_v2")

			live, err := eng.Inspect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if changes, _, err := migrate.Diff(live, v2, migrate.DiffOptions{}); err != nil || len(changes) != 0 {
				t.Fatalf("after the change the diff must be empty: %+v %v", changes, err)
			}
			r := myRunner{t, db}
			if err := r.Exec(`INSERT INTO fk_parents (id) VALUES (600)`); err != nil {
				t.Fatal(err)
			}
			if err := r.Exec(`INSERT INTO fk_del_cascade (id, parent_id) VALUES (7, 600)`); err != nil {
				t.Fatal(err)
			}
			if err := r.Exec(`DELETE FROM fk_parents WHERE id = 600`); err != nil {
				t.Fatal(err)
			}
			if got := r.Scalar(`SELECT COALESCE(CAST(parent_id AS CHAR), 'NULL') FROM fk_del_cascade WHERE id = 7`); got != "NULL" {
				t.Fatalf("the changed key must SET NULL, the child has parent_id %s", got)
			}
		})
	}
}
