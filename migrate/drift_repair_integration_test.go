// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
)

// driftRefs spells a list of object references as "kind name" strings.
func driftRefs(refs []migrate.ObjectRef) []string {
	var out []string
	for _, r := range refs {
		out = append(out, r.Kind+" "+r.Name)
	}
	return out
}

// applyStatements runs the statements of a repair one by one on the live
// database, as a person who reviewed the script would.
func (f *driftFixture) applyStatements(stmts []migrate.Statement) {
	f.t.Helper()
	for _, s := range stmts {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(s.SQL)), "PRAGMA FOREIGN_KEY_CHECK") {
			continue
		}
		f.exec(s.SQL)
	}
}

// Catches: drift on a database without history listing every table as
// unmanaged (or nothing), instead of comparing it with the latest snapshot and
// naming the differing column and constraints with both definitions.
func TestDriftWithoutHistoryNamesTheDifferingObjects(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		f := newDriftFixture(t, db, d)
		rep, err := f.engine().Drift(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Changed) != 1 || rep.Changed[0].Object.Kind != "column" || rep.Changed[0].Object.Name != "items.status" {
			t.Fatalf("changed = %+v, want exactly column items.status", rep.Changed)
		}
		want := []string{"check items.items_kind_check", "check items.items_status_check"}
		if got := driftRefs(rep.Missing); !slices.Equal(got, want) {
			t.Fatalf("missing = %v, want %v", got, want)
		}
		if len(rep.Unmanaged) != 0 {
			t.Fatalf("unmanaged = %v, want none: the tables are known from the snapshot", driftRefs(rep.Unmanaged))
		}
		c := rep.Changed[0]
		if !strings.Contains(c.Detail, "default") || !strings.Contains(c.Detail, "nullable") {
			t.Errorf("detail %q does not say that the default and nullability differ", c.Detail)
		}
		if !strings.Contains(c.Live, "nullable=true") || !strings.Contains(c.Live, "default=none") {
			t.Errorf("live definition %q, want a nullable column without default", c.Live)
		}
		if !strings.Contains(c.Snapshot, "nullable=false") || !strings.Contains(c.Snapshot, "new") {
			t.Errorf("snapshot definition %q, want a NOT NULL column defaulting to 'new'", c.Snapshot)
		}
		if line := c.Line(); !strings.Contains(line, "column items.status") || !strings.Contains(line, "live: ") || !strings.Contains(line, "snapshot: ") {
			t.Errorf("line %q does not name the object with both definitions", line)
		}
	})
}

// Catches: the refusal naming the objects only as text, so that a caller cannot
// read them, and the refusal losing errors.Is(ErrBaselineMismatch).
func TestBaselineMismatchErrorCarriesTheReport(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		f := newDriftFixture(t, db, d)
		err := f.engine().Baseline(context.Background(), f.version, false)
		if !errors.Is(err, migrate.ErrBaselineMismatch) {
			t.Fatalf("baseline: %v, want ErrBaselineMismatch", err)
		}
		var me *migrate.BaselineMismatchError
		if !errors.As(err, &me) {
			t.Fatalf("the refusal %T is not a *BaselineMismatchError", err)
		}
		if len(me.Report.Changed) != 1 || len(me.Report.Missing) != 2 {
			t.Fatalf("report = %+v, want 1 changed and 2 missing", me.Report)
		}
		for _, want := range []string{"changed: column items.status", "nullable=true", "missing: check items.items_status_check", "missing: check items.items_kind_check"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal lacks %q:\n%v", want, err)
			}
		}
	})
}

// Catches: a repair whose statements do not bring the live database to the
// snapshot (drift stays), that mixes in steps it was not asked for, or that
// renders SQL a dialect cannot run.
func TestDriftRepairBringsTheDatabaseToTheSnapshot(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		f := newDriftFixture(t, db, d)
		r, err := f.engine().DriftRepair(ctx, migrate.RepairOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !r.Complete() || len(r.Statements) == 0 {
			t.Fatalf("repair = %+v, want statements and nothing withheld", r)
		}
		sql := r.SQL()
		for _, want := range []string{"items_status_check", "items_kind_check"} {
			if !strings.Contains(sql, want) {
				t.Errorf("the script does not restore %s:\n%s", want, sql)
			}
		}
		if strings.Contains(strings.ToUpper(sql), "DROP TABLE ITEMS") && d != dialect.SQLite {
			t.Errorf("the script drops the items table:\n%s", sql)
		}
		f.applyStatements(r.Statements)
		rep, err := f.engine().Drift(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !rep.Clean() {
			t.Fatalf("drift after the repair: %+v\nscript:\n%s", rep, sql)
		}
		again, err := f.engine().DriftRepair(ctx, migrate.RepairOptions{})
		if err != nil || len(again.Statements) != 0 || !again.Complete() {
			t.Fatalf("a clean database needs no repair: %+v, %v", again, err)
		}
		if err := f.engine().Baseline(ctx, f.version, false); err != nil {
			t.Fatalf("baseline after the repair: %v", err)
		}
	})
}

// Catches: a repair that prints a destructive step (here the drop of a table
// the snapshot does not know) without being asked, or that hides it instead of
// listing it, or that cannot be asked for it.
func TestDriftRepairWithholdsDestructiveSteps(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		f := newDriftFixture(t, db, d)
		f.exec(`CREATE TABLE scratch (id integer)`)

		r, err := f.engine().DriftRepair(ctx, migrate.RepairOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if r.Complete() || len(r.Withheld) != 1 || r.Withheld[0].Change.Kind != migrate.DropTable || r.Withheld[0].Change.Name != "scratch" {
			t.Fatalf("withheld = %+v, want the drop of table scratch", r.Withheld)
		}
		if strings.Contains(strings.ToUpper(r.SQL()), "SCRATCH") {
			t.Fatalf("the script mentions the withheld table:\n%s", r.SQL())
		}
		if len(r.Statements) == 0 {
			t.Fatal("the harmless steps are still printed")
		}

		r, err = f.engine().DriftRepair(ctx, migrate.RepairOptions{AllowDestructive: true})
		if err != nil {
			t.Fatal(err)
		}
		if !r.Complete() || !strings.Contains(strings.ToUpper(r.SQL()), "DROP TABLE") {
			t.Fatalf("with AllowDestructive the drop is printed and nothing is withheld: %+v\n%s", r.Withheld, r.SQL())
		}
		f.applyStatements(r.Statements)
		rep, err := f.engine().Drift(ctx)
		if err != nil || !rep.Clean() {
			t.Fatalf("drift after the destructive repair: %+v, %v", rep, err)
		}
	})
}
