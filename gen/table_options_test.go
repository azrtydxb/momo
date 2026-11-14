// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"
)

// createSQL renders the CREATE TABLE statements a SQLite database gets for s.
func createSQL(t *testing.T, s *schema.Schema) string {
	t.Helper()
	empty := &schema.Schema{Dialect: dialect.SQLite}
	changes, _, err := migrate.Diff(empty, s, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := msqlite.New().RenderSchema(empty, s, changes)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, st := range stmts {
		sb.WriteString(st.SQL + "\n")
	}
	return sb.String()
}

// Catches: a momo.Model table option (autoincrement, without_rowid, strict)
// that is parsed but never reaches the schema or the CREATE TABLE statement,
// so a model written by --from-db builds a table without the option.
func TestTableOptionsReachCreateTable(t *testing.T) {
	auto := mdl("Counter", "counters", fld("ID", "int64", "pk,identity"), fld("Name", "string", ""))
	auto.Autoincrement, auto.Strict = true, true
	wor := mdl("Setting", "settings", fld("K", "string", "pk"), fld("V", "string", "nullable"))
	wor.WithoutRowID, wor.Strict = true, true
	s, diags := build(t, sqlCfg(), auto, wor)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if tb, _ := s.Table("counters"); !tb.Autoincrement || !tb.Strict || tb.WithoutRowID {
		t.Errorf("counters = %+v", tb)
	}
	if tb, _ := s.Table("settings"); tb.Autoincrement || !tb.Strict || !tb.WithoutRowID {
		t.Errorf("settings = %+v", tb)
	}
	sql := createSQL(t, s)
	for _, want := range []string{"PRIMARY KEY AUTOINCREMENT", "WITHOUT ROWID, STRICT", ") STRICT"} {
		if !strings.Contains(sql, want) {
			t.Errorf("the DDL lacks %q:\n%s", want, sql)
		}
	}
}

// Catches: autoincrement accepted on a table SQLite cannot make AUTOINCREMENT
// (composite key, non-integer key, WITHOUT ROWID), which would fail only when
// the migration runs, and the options accepted on a dialect that has none.
func TestTableOptionsAreChecked(t *testing.T) {
	for name, c := range map[string]struct {
		m    *Model
		cfg  Config
		want string
	}{
		"composite key": {mdl("A", "a", fld("X", "int64", "pk"), fld("Y", "int64", "pk")), sqlCfg(), "exactly one column"},
		"text key":      {mdl("A", "a", fld("ID", "string", "pk")), sqlCfg(), "needs an INTEGER primary key"},
		"no key":        {mdl("A", "a", fld("ID", "int64", "")), sqlCfg(), "exactly one column"},
		"without rowid": {mdl("A", "a", fld("ID", "int64", "pk")), sqlCfg(), "cannot be combined with without_rowid"},
		"postgres":      {mdl("A", "a", fld("ID", "int64", "pk")), pg(), "SQLite table options"},
	} {
		c.m.Autoincrement = true
		c.m.WithoutRowID = name == "without rowid"
		if _, diags := build(t, c.cfg, c.m); !strings.Contains(diagMsgs(diags), c.want) {
			t.Errorf("%s: diagnostics = %q, want %q", name, diagMsgs(diags), c.want)
		}
	}
	nokey := mdl("A", "a", fld("ID", "int64", ""))
	nokey.WithoutRowID = true
	if _, diags := build(t, sqlCfg(), nokey); !strings.Contains(diagMsgs(diags), "without_rowid needs a primary key") {
		t.Errorf("diagnostics = %q", diagMsgs(diags))
	}
}

// Catches: the table options being read from a model's momo.Model tag only on
// paper, so a real package never gets them, and an option written on a field,
// which means nothing, being ignored without a word.
func TestTableOptionsFromSource(t *testing.T) {
	cfg := Config{Dir: fixturesDir(t), Dialects: []dialect.Name{dialect.SQLite}}
	g, diags, err := Load([]string{"./tableoptions"}, cfg)
	if err != nil || len(diags) != 0 {
		t.Fatalf("err=%v diags=%v", err, diags)
	}
	counters, _ := g.Schema.Table("counters")
	settings, _ := g.Schema.Table("settings")
	if !counters.Autoincrement || !counters.Strict || counters.WithoutRowID || !settings.WithoutRowID || settings.Strict {
		t.Fatalf("counters = %+v, settings = %+v", counters, settings)
	}
	_, diags, err = Load([]string{"./tableoptionsbad"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var all []string
	for _, d := range diags {
		all = append(all, d.String())
	}
	joined := strings.Join(all, "\n")
	if strings.Count(joined, "are options of the momo.Model tag, not of a field") != 2 {
		t.Errorf("want one diagnostic per misplaced field tag:\n%s", joined)
	}
}
