// SPDX-License-Identifier: Apache-2.0

// Package fkcase builds the schemas of the foreign key action tests of every
// migration dialect from models, through the generator's own schema builder, so
// the tests run the path a user's model takes: the on_delete, on_update and
// no_constraint tags become foreign keys (or none) before the diff sees them.
// It is for tests only.
package fkcase

import (
	"fmt"
	"go/token"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/schema"
)

// Case is one child table whose foreign key carries the given actions, spelled
// as the on_delete and on_update tag values are ("cascade", "set_null",
// "restrict", "no_action", "set_default"). An empty action leaves the clause
// out, which is the database default (NO ACTION).
type Case struct{ Name, OnDelete, OnUpdate string }

// Parent is the referenced table; Loose is the table whose relation carries
// the no_constraint tag. A case Name "x" is the table "fk_x".
const (
	Parent = "fk_parents"
	Loose  = "fk_loose"
)

// Cases lists every action on delete and on update. setDefault adds SET
// DEFAULT, which MySQL and MariaDB reject.
func Cases(setDefault bool) []Case {
	out := []Case{
		{"del_default", "", ""},
		{"del_noaction", "no_action", ""}, {"del_restrict", "restrict", ""}, {"del_cascade", "cascade", ""}, {"del_setnull", "set_null", ""},
		{"upd_noaction", "", "no_action"}, {"upd_restrict", "", "restrict"}, {"upd_cascade", "", "cascade"}, {"upd_setnull", "", "set_null"},
	}
	if setDefault {
		out = append(out, Case{"del_setdefault", "set_default", ""}, Case{"upd_setdefault", "", "set_default"})
	}
	return out
}

func field(name, goType, tag string, i int) *gen.Field {
	tg, _ := tags.Parse(tag)
	col := "id"
	if name == "ParentID" {
		col = "parent_id"
	}
	return &gen.Field{
		GoName: name, Path: []int{i + 1}, Column: col, GoType: goType, BaseType: "int64",
		Nullable: goType[0] == '*', Tags: tg, Pos: token.Position{Filename: "m.go", Line: 10 + i, Column: 2},
	}
}

func model(goName, table string) *gen.Model {
	return &gen.Model{
		GoName: goName, PkgPath: "ex/m", Table: table,
		Fields: []*gen.Field{field("ID", "int64", "pk", 0), field("ParentID", "*int64", "", 1)},
		Pos:    token.Position{Filename: "m.go", Line: 3, Column: 6},
		Config: &gen.CompanionResult{
			Defaults: map[string]string{"parent_id": "0"}, Comments: map[string]string{}, Collates: map[string]string{},
		},
	}
}

// Schema builds the schema of a parent table, one child table per case and a
// table whose relation has no database constraint, for dialect d. It fails on
// any diagnostic of the schema builder.
func Schema(d dialect.Name, cases []Case) (*schema.Schema, error) {
	parent := model("FKParent", Parent)
	parent.Fields = parent.Fields[:1]
	models := []*gen.Model{parent}
	rel := func(m *gen.Model, onDelete, onUpdate string, loose bool) {
		m.Relations = []*gen.Relation{{
			GoName: "Parent", Kind: gen.BelongsTo, Target: parent, FK: "ParentID",
			OnDelete: onDelete, OnUpdate: onUpdate, NoConstraint: loose,
			Pos: token.Position{Filename: "m.go", Line: 40, Column: 2},
		}}
	}
	for i, c := range cases {
		m := model(fmt.Sprintf("FKChild%d", i), "fk_"+c.Name)
		rel(m, c.OnDelete, c.OnUpdate, false)
		models = append(models, m)
	}
	loose := model("FKLoose", Loose)
	rel(loose, "", "", true)
	models = append(models, loose)

	s, diags := gen.BuildSchema(models, gen.Config{Dialects: []dialect.Name{d}})
	if len(diags) != 0 {
		return nil, fmt.Errorf("fkcase: schema diagnostics: %v", diags)
	}
	return s, nil
}
