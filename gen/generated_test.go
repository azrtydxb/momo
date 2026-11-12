// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"
)

func readOnlyField(name, goType, tag string) *Field {
	f := fld(name, goType, tag)
	f.Access = ReadOnly
	return f
}

func diagMsgs(diags []Diagnostic) string {
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Msg)
	}
	return strings.Join(msgs, "\n")
}

// Catches: the generated tag parsed but never reaching schema.Column, so the
// snapshot and migrate lose the GENERATED ALWAYS AS expression.
func TestGeneratedTagReachesColumn(t *testing.T) {
	s, diags := build(t, pg(), mdl("Thing", "things", fld("Name", "string", ""),
		readOnlyField("Normalized", "string", "read_only,generated:lower(name)")))
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	col, ok := s.Tables[0].Column("normalized")
	if !ok || col.Generated != "lower(name)" {
		t.Fatalf("column = %+v, want the generated expression", col)
	}
}

// Catches: Config.Generated read but not merged, so an expression with a comma
// never reaches the column.
func TestGeneratedCompanionReachesColumn(t *testing.T) {
	m := mdl("Thing", "things", fld("Name", "string", ""), readOnlyField("Label", "string", "read_only"))
	m.Config.Generated = map[string]string{"label": "coalesce(name, '')"}
	s, diags := build(t, pg(), m)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if col, ok := s.Tables[0].Column("label"); !ok || col.Generated != "coalesce(name, '')" {
		t.Fatalf("column = %+v, want the companion expression", col)
	}
}

// Catches: a generated column that is writable, which Insert and Update would
// write and the database would refuse, or one that shares a column with a tag
// or companion call that decides the value some other way.
func TestGeneratedTagRefusals(t *testing.T) {
	cases := []struct {
		name  string
		tags  string
		setup func(*Model)
		want  string
	}{
		{name: "writable", tags: "generated:lower(name)", want: "must be read_only"},
		{name: "default", tags: "read_only,generated:lower(name),default:'x'", want: "generated conflicts with default"},
		{name: "identity", tags: "read_only,generated:lower(name),identity", want: "generated conflicts with identity"},
		{name: "serial", tags: "read_only,generated:lower(name),serial", want: "generated conflicts with serial"},
		{name: "dbdefault", tags: "read_only,generated:lower(name),dbdefault", want: "generated conflicts with dbdefault"},
		{name: "version", tags: "read_only,generated:lower(name),version", want: "generated conflicts with version"},
		{name: "soft_delete", tags: "read_only,generated:lower(name),soft_delete:timestamp", want: "generated conflicts with soft_delete"},
		{name: "created_at", tags: "read_only,generated:lower(name),created_at", want: "generated conflicts with created_at"},
		{name: "updated_at", tags: "read_only,generated:lower(name),updated_at", want: "generated conflicts with updated_at"},
		{name: "idgen tag", tags: "read_only,generated:lower(name),idgen:cuid", want: "generated conflicts with idgen"},
		{
			name: "idgen config", tags: "read_only,generated:lower(name)", want: "generated conflicts with idgen",
			setup: func(m *Model) { m.Config.IDGens = map[string]string{"normalized": "cuid"} },
		},
		{
			name: "tag and config", tags: "read_only,generated:lower(name)", want: "set by both the tag and MomoConfig",
			setup: func(m *Model) { m.Config.Generated = map[string]string{"normalized": "upper(name)"} },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fld("Normalized", "string", tc.tags)
			if strings.Contains(tc.tags, "read_only") {
				f.Access = ReadOnly
			}
			m := mdl("Thing", "things", f)
			if tc.setup != nil {
				tc.setup(m)
			}
			_, diags := build(t, pg(), m)
			if got := diagMsgs(diags); !strings.Contains(got, tc.want) {
				t.Fatalf("diagnostics %q lack %q", got, tc.want)
			}
		})
	}
}
