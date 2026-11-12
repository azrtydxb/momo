// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func myCfg() Config  { return Config{Dialects: []dialect.Name{dialect.MySQL}} }
func sqlCfg() Config { return Config{Dialects: []dialect.Name{dialect.SQLite}} }

// Catches: the virtual tag parsed but never reaching schema.Column, so the
// model declares STORED and migrate renders STORED.
func TestVirtualTagReachesColumn(t *testing.T) {
	for _, cfg := range []Config{myCfg(), sqlCfg()} {
		s, diags := build(t, cfg, mdl("Thing", "things", fld("Name", "string", "type:varchar(255)"),
			readOnlyField("Normalized", "string", "read_only,type:varchar(255),generated:lower(name),virtual")))
		if len(diags) != 0 {
			t.Fatalf("%v: diagnostics: %v", cfg.Dialects, diags)
		}
		col, ok := s.Tables[0].Column("normalized")
		if !ok || col.Generated != "lower(name)" || !col.GeneratedVirtual {
			t.Fatalf("%v: column = %+v, want a VIRTUAL generated column", cfg.Dialects, col)
		}
	}
	s, _ := build(t, myCfg(), mdl("Thing", "things", fld("Name", "string", "type:varchar(255)"),
		readOnlyField("Normalized", "string", "read_only,type:varchar(255),generated:lower(name)")))
	if col, _ := s.Tables[0].Column("normalized"); col.GeneratedVirtual {
		t.Fatal("a generated column without virtual must stay STORED")
	}
}

// Catches: the MomoConfig hook ignored, so an expression with a comma cannot
// be VIRTUAL.
func TestVirtualCompanionReachesColumn(t *testing.T) {
	m := mdl("Thing", "things", fld("Name", "string", "type:varchar(255)"), readOnlyField("Label", "string", "read_only,type:varchar(255)"))
	m.Config.Generated = map[string]string{"label": "coalesce(name, '')"}
	m.Config.GeneratedVirtual = map[string]bool{"label": true}
	s, diags := build(t, myCfg(), m)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if col, ok := s.Tables[0].Column("label"); !ok || col.Generated != "coalesce(name, '')" || !col.GeneratedVirtual {
		t.Fatalf("column = %+v, want the VIRTUAL companion expression", col)
	}
}

// Catches: virtual accepted on a column with no expression, which would write
// a flag that means nothing.
func TestVirtualNeedsGenerated(t *testing.T) {
	_, diags := build(t, myCfg(), mdl("Thing", "things", fld("Name", "string", "type:varchar(255),virtual")))
	if !strings.Contains(diagMsgs(diags), "virtual is valid only together with generated") {
		t.Fatalf("diagnostics = %q", diagMsgs(diags))
	}
}

// Catches: a VIRTUAL column generated for PostgreSQL, which has them only from
// version 18 and cannot apply the migration on 16 or 17.
func TestVirtualRefusedOnPostgres(t *testing.T) {
	_, diags := build(t, pg(), mdl("Thing", "things", fld("Name", "string", "type:varchar(255)"),
		readOnlyField("Normalized", "string", "read_only,type:varchar(255),generated:lower(name),virtual")))
	if !strings.Contains(diagMsgs(diags), "virtual is not supported on postgres") {
		t.Fatalf("diagnostics = %q", diagMsgs(diags))
	}
}

// Catches: the declared VIRTUAL column differing from the inspected one, so a
// database baselined with a VIRTUAL column gets a spurious migrate diff.
func TestVirtualModelDiffsCleanAgainstInspected(t *testing.T) {
	declared, diags := build(t, myCfg(), mdl("Thing", "things", fld("Name", "string", "type:varchar(255)"),
		readOnlyField("Normalized", "string", "read_only,type:varchar(255),generated:lower(name),virtual")))
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	got, _ := declared.Tables[0].Column("normalized")
	one := func(c schema.Column) *schema.Schema {
		return &schema.Schema{Dialect: dialect.MySQL, Tables: []schema.Table{{Name: "things", Columns: []schema.Column{c}}}}
	}
	// What the MySQL inspector reports for the VIRTUAL column.
	inspected := schema.Column{Name: "normalized", Type: got.Type, Nullable: got.Nullable, Generated: "lower(name)", GeneratedVirtual: true}
	changes, _, err := migrate.Diff(one(inspected), one(*got), migrate.DiffOptions{})
	if err != nil || len(changes) != 0 {
		t.Fatalf("diff of the inspected against the declared column: %v, %+v", err, changes)
	}
	// The same column read as STORED (the old inspector) is a change.
	inspected.GeneratedVirtual = false
	changes, _, err = migrate.Diff(one(inspected), one(*got), migrate.DiffOptions{})
	if err != nil || len(changes) != 1 {
		t.Fatalf("STORED against VIRTUAL: %v, %+v", err, changes)
	}
}
