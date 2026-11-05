// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// renameFixture returns two tables: orgs(id, code) and users(id, org_id, email)
// with a primary key, unique, index, check and FK mentioning the columns that
// the tests rename.
func renameFixture() *schema.Schema {
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{
		{
			Name:       "orgs",
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "code", Type: "text"}},
			PrimaryKey: []string{"id"},
			Uniques:    []schema.Unique{{Name: "orgs_code_key", Columns: []string{"code"}}},
		},
		{
			Name: "users",
			Columns: []schema.Column{
				{Name: "uid", Type: "bigint"}, {Name: "org_id", Type: "bigint"}, {Name: "email", Type: "text"},
			},
			PrimaryKey: []string{"uid"},
			Uniques:    []schema.Unique{{Name: "users_email_key", Columns: []string{"email"}}},
			Indexes:    []schema.Index{{Name: "users_email_idx", Parts: []schema.IndexPart{{Column: "email"}}, Where: "email is not null"}},
			Checks:     []schema.Check{{Name: "users_email_chk", Expr: "email <> ''"}},
			ForeignKeys: []schema.ForeignKey{{
				Name: "users_org_fk", Columns: []string{"org_id"}, RefTable: "orgs", RefColumns: []string{"id"},
			}},
		},
	}}
}

func renameColumn(s *schema.Schema, table, from, to string) {
	for i := range s.Tables {
		t := &s.Tables[i]
		if t.Name != table {
			continue
		}
		fix := func(cols []string) {
			for j, c := range cols {
				if c == from {
					cols[j] = to
				}
			}
		}
		for j := range t.Columns {
			if t.Columns[j].Name == from {
				t.Columns[j].Name = to
			}
		}
		fix(t.PrimaryKey)
		for j := range t.Uniques {
			fix(t.Uniques[j].Columns)
		}
		for j := range t.Indexes {
			for k := range t.Indexes[j].Parts {
				if t.Indexes[j].Parts[k].Column == from {
					t.Indexes[j].Parts[k].Column = to
				}
			}
			t.Indexes[j].Where = strings.ReplaceAll(t.Indexes[j].Where, from, to)
		}
		for j := range t.Checks {
			t.Checks[j].Expr = strings.ReplaceAll(t.Checks[j].Expr, from, to)
		}
		for j := range t.ForeignKeys {
			fix(t.ForeignKeys[j].Columns)
		}
	}
	for i := range s.Tables {
		for j := range s.Tables[i].ForeignKeys {
			fk := &s.Tables[i].ForeignKeys[j]
			if fk.RefTable == table {
				for k, c := range fk.RefColumns {
					if c == from {
						fk.RefColumns[k] = to
					}
				}
			}
		}
	}
}

func TestRenamedColumnRemapsConstraints(t *testing.T) {
	cases := map[string]struct{ table, from, to string }{
		"primary key": {"users", "uid", "id"},
		"unique":      {"orgs", "code", "slug"},
		"index":       {"users", "email", "mail"}, // also unique, check, partial index predicate
		"fk local":    {"users", "org_id", "organization_id"},
		"fk ref":      {"orgs", "id", "oid"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			from := renameFixture()
			to := renameFixture()
			renameColumn(to, c.table, c.from, c.to)
			opts := DiffOptions{RenameColumns: map[string]string{c.table + "." + c.from: c.to}}
			changes, hints, err := Diff(from, to, opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := kinds(changes); !reflect.DeepEqual(got, []ChangeKind{RenameColumn}) {
				t.Fatalf("a rename must yield only rename_column, got %v", got)
			}
			if len(hints) != 0 {
				t.Fatalf("unexpected hints %v", hints)
			}
		})
	}
}

func TestRenamedTableRemapsForeignKeys(t *testing.T) {
	from := renameFixture()
	to := renameFixture()
	to.Tables[0].Name = "organizations"
	to.Tables[1].ForeignKeys[0].RefTable = "organizations"
	changes, _, err := Diff(from, to, DiffOptions{RenameTables: map[string]string{"orgs": "organizations"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(changes); !reflect.DeepEqual(got, []ChangeKind{RenameTable}) {
		t.Fatalf("a table rename must yield only rename_table, got %v", got)
	}
}

func twoSchemas() *schema.Schema {
	col := []schema.Column{{Name: "id", Type: "bigint"}}
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{
		{Schema: "a", Name: "users", Columns: col},
		{Schema: "b", Name: "users", Columns: col},
	}}
}

func TestRenameTableIsSchemaQualified(t *testing.T) {
	from := twoSchemas()
	to := twoSchemas()
	to.Tables[0].Name = "people"
	if _, _, err := Diff(from, to, DiffOptions{RenameTables: map[string]string{"users": "people"}}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("an unqualified name in two schemas must be ambiguous, got %v", err)
	}
	for i := 0; i < 20; i++ {
		changes, _, err := Diff(from, to, DiffOptions{RenameTables: map[string]string{"a.users": "people"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(changes) != 1 || changes[0].Kind != RenameTable || changes[0].Schema != "a" || changes[0].Table != "users" || changes[0].Name != "people" {
			t.Fatalf("only a.users must be renamed, got %+v", changes)
		}
	}
	// The other schema's table is untouched and still diffs as unchanged.
	if _, _, err := Diff(from, to, DiffOptions{RenameTables: map[string]string{"b.users": "people"}}); err == nil {
		t.Fatal("b.users renames to a table that does not exist in schema b; the flag must not match")
	}
}

func TestDroppedTablesDropTheirForeignKeys(t *testing.T) {
	col := []schema.Column{{Name: "id", Type: "bigint"}, {Name: "other", Type: "bigint"}}
	from := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{
		{Name: "a", Columns: col, ForeignKeys: []schema.ForeignKey{{Name: "a_b", Columns: []string{"other"}, RefTable: "b", RefColumns: []string{"id"}}}},
		{Name: "b", Columns: col, ForeignKeys: []schema.ForeignKey{{Name: "b_a", Columns: []string{"other"}, RefTable: "a", RefColumns: []string{"id"}}}},
	}}
	changes, _, err := Diff(from, &schema.Schema{Dialect: "postgres"}, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []ChangeKind{DropFK, DropTable, DropFK, DropTable}
	if got := kinds(changes); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	if changes[0].Table != "a" || changes[0].Name != "a_b" || changes[0].Destructive {
		t.Fatalf("unexpected drop_fk %+v", changes[0])
	}
}

func TestDiffOutputIsByteEqual(t *testing.T) {
	from := renameFixture()
	to := renameFixture()
	renameColumn(to, "users", "email", "mail")
	to.Tables = append(to.Tables, schema.Table{Name: "zz", Columns: []schema.Column{{Name: "id", Type: "bigint"}}})
	to.Enums = []schema.Enum{{Name: "e", Values: []string{"a", "b"}}}
	to.Views = []schema.View{{Name: "v", Query: "select 1"}}
	var first []byte
	for i := 0; i < 20; i++ {
		changes, hints, err := Diff(from, to, DiffOptions{AllowDestructive: true})
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(struct {
			C []Change
			H []string
		}{changes, hints})
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = b
		} else if !bytes.Equal(first, b) {
			t.Fatalf("run %d differs:\n%s\n%s", i, first, b)
		}
	}
}

func TestDiffUnsupportedAlterations(t *testing.T) {
	t.Run("enum value removal", func(t *testing.T) {
		from := base()
		from.Enums = []schema.Enum{{Name: "mood", Values: []string{"ok", "bad"}}}
		to := base()
		to.Enums = []schema.Enum{{Name: "mood", Values: []string{"ok"}}}
		if _, _, err := Diff(from, to, DiffOptions{AllowDestructive: true}); err == nil || !strings.Contains(err.Error(), `"bad"`) {
			t.Fatalf("want an error naming the removed value, got %v", err)
		}
	})
	t.Run("partitioning", func(t *testing.T) {
		from := base()
		to := base()
		to.Tables[0].Partition = &schema.Partition{Strategy: "range", Key: "RANGE (id)"}
		if _, _, err := Diff(from, to, DiffOptions{AllowDestructive: true}); err == nil || !strings.Contains(err.Error(), "partitioning") {
			t.Fatalf("want a partitioning error, got %v", err)
		}
	})
	t.Run("without rowid", func(t *testing.T) {
		from := base()
		to := base()
		to.Tables[0].WithoutRowID = true
		if _, _, err := Diff(from, to, DiffOptions{AllowDestructive: true}); err == nil || !strings.Contains(err.Error(), "WITHOUT ROWID") {
			t.Fatalf("want a WITHOUT ROWID error, got %v", err)
		}
	})
}

func TestDiffSchemaExtensionRemovalIsHintOnly(t *testing.T) {
	from := base()
	from.Schemas = []schema.SchemaNamespace{{Name: "app"}}
	from.Extensions = []schema.Extension{{Name: "vector"}}
	changes, hints, err := Diff(from, base(), DiffOptions{})
	if err != nil || len(changes) != 0 {
		t.Fatalf("removal must emit no change and no error: %v %v", changes, err)
	}
	want := []string{
		"extension vector is no longer managed; it is not dropped",
		"schema app is no longer managed; it is not dropped",
	}
	if !reflect.DeepEqual(hints, want) {
		t.Fatalf("hints = %q, want %q", hints, want)
	}
}
