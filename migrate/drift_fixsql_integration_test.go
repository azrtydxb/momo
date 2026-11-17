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
	"github.com/azrtydxb/momo/schema"
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

// driftTypeMutations change the type of items.qty outside the migrations.
func driftTypeMutations(d dialect.Name) []string {
	switch d {
	case dialect.SQLite:
		return []string{
			`DROP TABLE items`,
			`CREATE TABLE items (id INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'new', kind TEXT, qty TEXT, PRIMARY KEY (id), CONSTRAINT items_status_check CHECK ("status" IN ('done', 'new')), CONSTRAINT items_kind_check CHECK ("kind" IN ('a', 'b')))`,
		}
	case dialect.MySQL:
		return []string{`ALTER TABLE items MODIFY qty bigint NULL`}
	}
	return []string{`ALTER TABLE items ALTER COLUMN qty TYPE bigint`}
}

// Catches: a column type change (which can lose or reject data) being printed
// without --allow-destructive, or being withheld without a reason that names
// it; the type change must come back with the flag and leave no drift.
func TestDriftRepairWithholdsAColumnTypeChange(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		f := newDriftFixtureOf(t, db, d, driftDesired(d), driftTypeMutations(d))
		r, err := f.engine().DriftRepair(ctx, migrate.RepairOptions{})
		if err != nil {
			t.Fatal(err)
		}
		var typed *migrate.Skipped
		for i := range r.Withheld {
			if c := r.Withheld[i].Change; c.Kind == migrate.AlterColumn && c.Name == "qty" {
				typed = &r.Withheld[i]
			}
		}
		if typed == nil || !strings.Contains(typed.Reason, "type") || r.Complete() {
			t.Fatalf("withheld = %+v, want the type change of qty with its reason and an incomplete repair", r.Withheld)
		}
		if strings.Contains(strings.ToLower(r.SQL()), "qty") {
			t.Fatalf("the script touches the withheld column:\n%s", r.SQL())
		}
		r, err = f.engine().DriftRepair(ctx, migrate.RepairOptions{AllowDestructive: true})
		if err != nil || !r.Complete() {
			t.Fatalf("with AllowDestructive the repair is complete: %+v, %v", r, err)
		}
		f.applyStatements(r.Statements)
		if rep, err := f.engine().Drift(ctx); err != nil || !rep.Clean() {
			t.Fatalf("drift after the repair: %+v, %v\n%s", rep, err, r.SQL())
		}
	})
}

// driftViewDesired is driftDesired with a view over items.
func driftViewDesired(d dialect.Name) *schema.Schema {
	s := driftDesired(d)
	v := schema.View{Name: "items_v", Query: "SELECT id, qty FROM items"}
	if d == dialect.SQLite {
		v.Depends = []string{"items"} // the SQLite inspector reports it
	}
	s.Views = []schema.View{v}
	return s
}

// Catches: a changed view (a drop and a create of one name) printing the CREATE
// while the DROP is withheld, which fails with "already exists" and, on MySQL,
// leaves the earlier statements applied. Only PostgreSQL compares a view's query
// text, so only there can a view change; on the other dialects a view that is
// gone from the database is created again.
func TestDriftRepairWithholdsTheCreateThatReplacesAView(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		mutation := `DROP VIEW items_v`
		if d == dialect.Postgres {
			mutation = `CREATE OR REPLACE VIEW items_v AS SELECT id, qty + 1 AS qty FROM items`
		}
		f := newDriftFixtureOf(t, db, d, driftViewDesired(d), []string{mutation})
		r, err := f.engine().DriftRepair(ctx, migrate.RepairOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if d != dialect.Postgres {
			if !r.Complete() || !strings.Contains(strings.ToUpper(r.SQL()), "CREATE VIEW") {
				t.Fatalf("a missing view is created again: %+v\n%s", r, r.SQL())
			}
		} else {
			var got []migrate.ChangeKind
			for _, w := range r.Withheld {
				got = append(got, w.Change.Kind)
			}
			if !slices.Equal(got, []migrate.ChangeKind{migrate.DropView, migrate.CreateView}) {
				t.Fatalf("withheld = %v, want the drop of the view and the create that replaces it", got)
			}
			if strings.Contains(strings.ToUpper(r.SQL()), "VIEW") {
				t.Fatalf("the script must not create the view while the drop is withheld:\n%s", r.SQL())
			}
			r, err = f.engine().DriftRepair(ctx, migrate.RepairOptions{AllowDestructive: true})
			if err != nil || !r.Complete() {
				t.Fatalf("with AllowDestructive the repair is complete: %+v, %v", r, err)
			}
		}
		f.applyStatements(r.Statements)
		if rep, err := f.engine().Drift(ctx); err != nil || !rep.Clean() {
			t.Fatalf("drift after the repair: %+v, %v\n%s", rep, err, r.SQL())
		}
	})
}

// driftTriggerDesired is a table with row-level security, a trigger function, a
// trigger with a WHEN condition and a policy, on PostgreSQL.
func driftTriggerDesired() *schema.Schema {
	return &schema.Schema{
		Dialect:    dialect.Postgres,
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{{
			Name:       "docs",
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: "text", Nullable: true}},
			PrimaryKey: []string{"id"},
			RLS:        &schema.RLS{Enabled: true},
		}},
		Functions: []schema.Function{
			{Name: "touch", Args: "", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: "BEGIN RETURN NEW; END;"},
		},
		Triggers: []schema.Trigger{{
			Table: "docs", Name: "docs_touch", Timing: "BEFORE", Events: []string{"INSERT"}, ForEachRow: true,
			When: "new.id > 0", Function: "touch()",
		}},
		Policies: []schema.Policy{{Table: "docs", Name: "docs_pos", Command: "SELECT", Permissive: true, Using: "docs.id > 0"}},
	}
}

// Catches: a changed trigger condition and a changed policy expression (each a
// drop and a create of one name) printing the CREATE while the DROP is
// withheld, which fails with "already exists". Against a real PostgreSQL: both
// pairs are withheld without --allow-destructive, and with it the script makes
// drift clean.
func TestDriftRepairWithholdsTheCreateThatReplacesATriggerAndAPolicy(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	f := newDriftFixtureOf(t, db, dialect.Postgres, driftTriggerDesired(), []string{
		`DROP TRIGGER docs_touch ON docs`,
		`CREATE TRIGGER docs_touch BEFORE INSERT ON docs FOR EACH ROW WHEN (NEW.id > 5) EXECUTE FUNCTION touch()`,
		`DROP POLICY docs_pos ON docs`,
		`CREATE POLICY docs_pos ON docs FOR SELECT USING (id > 5)`,
	})
	r, err := f.engine().DriftRepair(ctx, migrate.RepairOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range r.Withheld {
		got = append(got, string(w.Change.Kind)+" "+w.Change.Name)
	}
	slices.Sort(got)
	want := []string{"create_policy docs_pos", "create_trigger docs_touch", "drop_policy docs_pos", "drop_trigger docs_touch"}
	if r.Complete() || !slices.Equal(got, want) {
		t.Fatalf("withheld = %v (complete %v), want %v", got, r.Complete(), want)
	}
	if up := strings.ToUpper(r.SQL()); strings.Contains(up, "TRIGGER") || strings.Contains(up, "POLICY") {
		t.Fatalf("the script must not create either object while its drop is withheld:\n%s", r.SQL())
	}

	r, err = f.engine().DriftRepair(ctx, migrate.RepairOptions{AllowDestructive: true})
	if err != nil || !r.Complete() {
		t.Fatalf("with AllowDestructive the repair is complete: %+v, %v", r, err)
	}
	up := strings.ToUpper(r.SQL())
	for _, w := range []string{"DROP TRIGGER", "CREATE TRIGGER", "DROP POLICY", "CREATE POLICY"} {
		if !strings.Contains(up, w) {
			t.Errorf("the script lacks %s:\n%s", w, r.SQL())
		}
	}
	f.applyStatements(r.Statements)
	if rep, err := f.engine().Drift(ctx); err != nil || !rep.Clean() {
		t.Fatalf("drift after the repair: %+v, %v\n%s", rep, err, r.SQL())
	}
}

// Catches: drift and --fix-sql comparing with the latest snapshot while the
// adoption checks the init snapshot (baseline --version N), so that a directory
// with later migrations repairs the database to a state baseline then refuses.
func TestDriftRepairVersionSelectsTheSnapshot(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		f := newDriftFixture(t, db, d)
		later := driftDesired(d)
		later.Tables[0].Columns = append(later.Tables[0].Columns, schema.Column{Name: "note", Type: later.Tables[0].Columns[2].Type, Nullable: true})
		m, err := f.engine().Diff(ctx, later, "note", migrate.DiffOptions{})
		if err != nil || m == nil {
			t.Fatalf("diff: %v, %v", m, err)
		}
		for k, v := range m.Files {
			f.files[k] = v
		}

		latest, err := f.engine().Drift(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if latest.SnapshotVersion != m.Version || !slices.Contains(driftRefs(latest.Missing), "column items.note") {
			t.Fatalf("without --version the latest snapshot %s is compared: %+v", m.Version, latest)
		}

		opts := migrate.DriftOptions{Version: f.version}
		rep, err := f.engine().DriftWith(ctx, opts)
		if err != nil {
			t.Fatal(err)
		}
		if rep.SnapshotVersion != f.version || len(rep.Changed) != 1 || len(rep.Missing) != 2 {
			t.Fatalf("with Version %s: %+v, want the status column and two checks only", f.version, rep)
		}
		if _, err := f.engine().DriftWith(ctx, migrate.DriftOptions{Version: "29990101000000"}); !errors.Is(err, migrate.ErrNoSnapshot) {
			t.Fatalf("an unknown version: %v, want ErrNoSnapshot", err)
		}

		r, err := f.engine().DriftRepair(ctx, migrate.RepairOptions{DriftOptions: opts})
		if err != nil || !r.Complete() {
			t.Fatalf("repair: %+v, %v", r, err)
		}
		f.applyStatements(r.Statements)
		if err := f.engine().Baseline(ctx, f.version, false); err != nil {
			t.Fatalf("baseline --version %s after the repair: %v", f.version, err)
		}
	})
}
