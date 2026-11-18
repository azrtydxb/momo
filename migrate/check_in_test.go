// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// checkInSchema is a table with one value-list check over its kind column.
func checkInSchema(values ...string) *schema.Schema {
	return &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{{
		Name:       "notes",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "kind", Type: "text"}},
		PrimaryKey: []string{"id"},
		Checks: []schema.Check{{
			Name: "notes_kind_check",
			Expr: schema.CheckInExpr(dialect.Postgres, "kind", values),
			In:   &schema.CheckIn{Column: "kind", Values: slices.Clone(values)},
		}},
	}}}
}

func checkInDiff(t *testing.T, from, to *schema.Schema) []migrate.Change {
	t.Helper()
	changes, _, err := migrate.Diff(from, to, migrate.DiffOptions{})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	return changes
}

// Catches: a value list compared as text, so that the order of the values, a
// repeated value or the spelling of the stored expression becomes a drop and
// add of the constraint.
func TestCheckInDiffIsASetComparison(t *testing.T) {
	stale := checkInSchema("a", "b")
	stale.Tables[0].Checks[0].Expr = "kind in ('b', 'a')"
	cases := map[string]struct{ from, to *schema.Schema }{
		"reordered":        {checkInSchema("a", "b", "c"), checkInSchema("c", "a", "b")},
		"repeated value":   {checkInSchema("a", "b"), checkInSchema("a", "b", "a")},
		"another spelling": {stale, checkInSchema("a", "b")},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := checkInDiff(t, c.from, c.to); len(got) != 0 {
				t.Fatalf("equal sets produced changes: %+v", got)
			}
			if got := checkInDiff(t, c.to, c.from); len(got) != 0 {
				t.Fatalf("equal sets produced changes the other way: %+v", got)
			}
		})
	}
}

// Catches: a value list read back from PostgreSQL, which carries no In, diffed
// as a drop and add of the constraint, or Equal and Diff disagreeing on it.
func TestCheckInDiffAndEqualAgreeOnTheStoredForm(t *testing.T) {
	read := func(expr string) *schema.Schema {
		s := checkInSchema("a", "b")
		s.Tables[0].Checks[0] = schema.Check{Name: "notes_kind_check", Expr: expr}
		return s
	}
	cases := map[string]struct {
		stored *schema.Schema
		want   *schema.Schema
		same   bool
	}{
		"same set":     {read("(kind = ANY (ARRAY['b'::text, 'a'::text]))"), checkInSchema("a", "b"), true},
		"wider":        {read("(kind = ANY (ARRAY['a'::text, 'b'::text]))"), checkInSchema("a", "b", "c"), false},
		"other column": {read("(other = ANY (ARRAY['a'::text, 'b'::text]))"), checkInSchema("a", "b"), false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			changes, _, err := migrate.Diff(c.stored, c.want, migrate.DiffOptions{AllowDestructive: true})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(changes) == 0; got != c.same {
				t.Errorf("Diff reports no change = %v, want %v: %+v", got, c.same, changes)
			}
			if got := schema.Equal(c.stored, c.want); got != c.same {
				t.Errorf("Equal = %v, want %v", got, c.same)
			}
		})
	}
}

// Catches: Equal reconciling value lists after the expressions were normalized,
// which lowercases a MySQL column, so a mixed-case column is Equal=false while
// Diff, which reads the expression as written, reports no change.
func TestCheckInMixedCaseColumnAgreesInEqualAndDiff(t *testing.T) {
	table := func(c schema.Check) *schema.Schema {
		return &schema.Schema{Dialect: dialect.MySQL, Tables: []schema.Table{{
			Name:       "notes",
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "Status", Type: "varchar(40)"}},
			PrimaryKey: []string{"id"},
			Checks:     []schema.Check{c},
		}}}
	}
	declared := table(schema.Check{
		Name: "notes_status_check",
		Expr: schema.CheckInExpr(dialect.MySQL, "Status", []string{"a", "b"}),
		In:   &schema.CheckIn{Column: "Status", Values: []string{"a", "b"}},
	})
	stored := table(schema.Check{Name: "notes_status_check", Expr: "(`Status` in (_utf8mb4\\'b\\',_utf8mb4\\'a\\'))"})
	changes, _, err := migrate.Diff(stored, declared, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("Diff reports %+v, want none", changes)
	}
	if !schema.Equal(declared, stored) || !schema.Equal(stored, declared) {
		t.Error("Equal must agree with Diff on a mixed-case column")
	}
}

// Catches: an added value rendered as a drop and add of the whole object, or
// flagged destructive.
func TestCheckInDiffAddsValues(t *testing.T) {
	got := checkInDiff(t, checkInSchema("a", "b"), checkInSchema("c", "a", "b", "d"))
	if len(got) != 1 {
		t.Fatalf("want one change, got %+v", got)
	}
	c := got[0]
	if c.Kind != migrate.AlterCheckIn || c.Table != "notes" || c.Name != "notes_kind_check" {
		t.Fatalf("change = %+v", c)
	}
	if !slices.Equal(c.Added, []string{"c", "d"}) || len(c.Removed) != 0 || c.Destructive {
		t.Fatalf("added %v removed %v destructive %v, want [c d] [] false", c.Added, c.Removed, c.Destructive)
	}
	if !c.Risky {
		t.Errorf("an alter_check_in is a risky change, got %+v", c)
	}
}

// Catches: a removed value that needs no opt-in, and a refusal that does not say
// how to fix it.
func TestCheckInDiffRemovalIsDestructive(t *testing.T) {
	from, to := checkInSchema("a", "x"), checkInSchema("a")
	_, _, err := migrate.Diff(from, to, migrate.DiffOptions{})
	if !errors.Is(err, migrate.ErrDestructive) {
		t.Fatalf("err = %v, want ErrDestructive", err)
	}
	if !strings.Contains(err.Error(), "backfill rows holding 'x' first") {
		t.Fatalf("the refusal must say what to do: %v", err)
	}
	changes, hints, err := migrate.Diff(from, to, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != migrate.AlterCheckIn || !changes[0].Destructive || !slices.Equal(changes[0].Removed, []string{"x"}) || len(changes[0].Added) != 0 {
		t.Fatalf("changes = %+v", changes)
	}
	if !slices.ContainsFunc(hints, func(h string) bool { return strings.Contains(h, "backfill rows holding 'x' first") }) {
		t.Fatalf("hints = %v, want the backfill hint", hints)
	}
}

// Catches: two branches that add different values to one list, merged by text,
// producing anything but one change that adds both, or a result that depends on
// the order of the merged lines.
func TestCheckInTwoBranchesMergeToOneChange(t *testing.T) {
	base := checkInSchema("a")
	for name, merged := range map[string]*schema.Schema{
		"b then c": checkInSchema("a", "b", "c"),
		"c then b": checkInSchema("a", "c", "b"),
	} {
		t.Run(name, func(t *testing.T) {
			got := checkInDiff(t, base, merged)
			if len(got) != 1 || got[0].Kind != migrate.AlterCheckIn || !slices.Equal(got[0].Added, []string{"b", "c"}) {
				t.Fatalf("changes = %+v, want one alter_check_in adding b and c", got)
			}
		})
	}
}

// Catches: a check that gains or loses its value list, or moves to another
// column, being compared as sets instead of replaced.
func TestCheckInFallsBackToDropAndAdd(t *testing.T) {
	plain := checkInSchema("a")
	// A plain check is one whose expression is no value list: an expression
	// that is one is read as a list on the side that carries no In, because
	// that is how a database reports it (see TestCheckInDiffAndEqualAgreeOnTheStoredForm).
	plain.Tables[0].Checks[0].In = nil
	plain.Tables[0].Checks[0].Expr = "kind <> ''"
	other := checkInSchema("a")
	other.Tables[0].Columns = append(other.Tables[0].Columns, schema.Column{Name: "tag", Type: "text", Nullable: true})
	other.Tables[0].Checks[0].In.Column = "tag"
	other.Tables[0].Checks[0].Expr = schema.CheckInExpr(dialect.Postgres, "tag", []string{"a"})
	for name, c := range map[string]struct{ from, to *schema.Schema }{
		"list to plain": {checkInSchema("a", "b"), plain},
		"plain to list": {plain, checkInSchema("a", "b")},
		"other column":  {checkInSchema("a"), other},
	} {
		t.Run(name, func(t *testing.T) {
			var kinds []migrate.ChangeKind
			for _, ch := range checkInDiff(t, c.from, c.to) {
				if ch.Kind != migrate.AddColumn {
					kinds = append(kinds, ch.Kind)
				}
			}
			if !slices.Equal(kinds, []migrate.ChangeKind{migrate.DropCheck, migrate.AddCheck}) && !slices.Equal(kinds, []migrate.ChangeKind{migrate.AddCheck, migrate.DropCheck}) {
				t.Fatalf("kinds = %v, want a drop and an add", kinds)
			}
		})
	}
}

// Catches: a value list that cannot be rendered reaching a database.
func TestCheckInRefusesUnrenderableLists(t *testing.T) {
	missingColumn := checkInSchema("a")
	missingColumn.Tables[0].Checks[0].In.Column = "nope"
	enumColumn := checkInSchema("a")
	enumColumn.Tables[0].Columns[1].Enum = "kinds"
	enumColumn.Enums = []schema.Enum{{Name: "kinds", Values: []string{"a"}}}
	cases := map[string]*schema.Schema{
		"no values":      checkInSchema(),
		"backslash":      checkInSchema("a\\b"),
		"newline":        checkInSchema("a\nb"),
		"missing column": missingColumn,
		"enum column":    enumColumn,
	}
	for name, to := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := migrate.Diff(&schema.Schema{Dialect: dialect.Postgres}, to, migrate.DiffOptions{}); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// Catches: PostgreSQL writing the value change without NOT VALID, or leaving
// the validation in the transactional file.
func TestCheckInEngineWritesTwoFilesOnPostgres(t *testing.T) {
	ctx := context.Background()
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	m1, err := first.Diff(ctx, checkInSchema("a", "b"), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	e := offlineEngine(t, mapFS(m1.Files), time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC))
	m, err := e.Diff(ctx, checkInSchema("c", "a", "b"), "add_c", migrate.DiffOptions{})
	if err != nil || m == nil || m.Next == nil {
		t.Fatalf("diff = %v, %v, want a migration with a no-transaction tail", m, err)
	}
	up := string(m.Files[m.Version+"_add_c.up.sql"])
	for _, want := range []string{`DROP CONSTRAINT "notes_kind_check"`, `ADD CONSTRAINT "notes_kind_check" CHECK ("kind" IN ('a','b','c')) NOT VALID`} {
		if !strings.Contains(up, want) {
			t.Fatalf("the transactional file lacks %s:\n%s", want, up)
		}
	}
	if strings.Contains(up, "VALIDATE") {
		t.Fatalf("the validation belongs in the no-transaction file:\n%s", up)
	}
	tail := string(m.Files[m.Next.Version+"_add_c_no_tx.up.sql"])
	if !strings.Contains(tail, `VALIDATE CONSTRAINT "notes_kind_check"`) || !strings.Contains(tail, "-- momo:no-transaction") {
		t.Fatalf("the tail must validate outside a transaction:\n%s", tail)
	}
}

// Catches: a value removal that migrate lint does not report when the
// migration lacks the destructive directive, and one that it reports anyway
// when the directive is there.
func TestLintReportsCheckValueRemoval(t *testing.T) {
	ctx := context.Background()
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	m1, err := first.Diff(ctx, checkInSchema("a", "b"), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	e := offlineEngine(t, mapFS(m1.Files), time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC))
	m2, err := e.Diff(ctx, checkInSchema("a"), "drop_b", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m2 == nil {
		t.Fatal(m2, err)
	}
	dir := mapFS(m1.Files)
	for k, v := range m2.Files {
		dir[k] = &fstest.MapFile{Data: v}
	}
	removals := func(fsys fstest.MapFS) []migrate.Finding {
		t.Helper()
		got, err := offlineEngine(t, fsys, time.Now()).LintAll()
		if err != nil {
			t.Fatal(err)
		}
		return slices.DeleteFunc(got, func(f migrate.Finding) bool { return f.Rule != migrate.RuleCheckValueRemoval })
	}
	upName := m2.Version + "_drop_b.up.sql"
	up := string(dir[upName].Data)
	if !strings.Contains(up, "-- momo:destructive") {
		t.Fatalf("a generated removal must carry the destructive directive:\n%s", up)
	}
	if got := removals(dir); len(got) != 0 {
		t.Fatalf("a reviewed removal was reported: %+v", got)
	}

	dir[upName] = &fstest.MapFile{Data: []byte(strings.ReplaceAll(up, "-- momo:destructive\n", ""))}
	sum, err := migrate.ComputeSum(dir)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := sum.Write(&buf); err != nil {
		t.Fatal(err)
	}
	dir[migrate.SumFileName] = &fstest.MapFile{Data: buf.Bytes()}
	got := removals(dir)
	if len(got) != 1 || got[0].Version != m2.Version || got[0].Severity != migrate.SeverityError || got[0].Alternative == "" || !strings.Contains(got[0].Message, "'b'") {
		t.Fatalf("findings = %+v, want one error naming 'b' on %s", got, m2.Version)
	}
}
