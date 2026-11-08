// SPDX-License-Identifier: Apache-2.0

package ent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/importer"
)

const fixtureDir = "../../internal/compat/entfixture/ent/schema"

// imports is the import block every inline schema in these tests starts with.
// The reader never compiles the source, so unused imports are fine.
const imports = `package schema

import (
	"time"

	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"entgo.io/ent/schema/mixin"
	"github.com/google/uuid"
)

`

var sourcePosition = regexp.MustCompile(`^[a-z]+\.go:\d+:\d+$`)

func TestReadReportsImplicitSaveWithPosition(t *testing.T) {
	_, rep, err := Read(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range rep.Changes {
		if c.Kind != importer.ImplicitSave || c.Model != "User" || c.Field != "Org" {
			continue
		}
		found = true
		if !strings.HasPrefix(c.Source, "user.go:") || !sourcePosition.MatchString(c.Source) {
			t.Fatalf("source must be user.go:<line>:<col> relative to the directory, got %q", c.Source)
		}
	}
	if !found {
		t.Fatalf("implicit-save for User.Org not reported: %s", rep)
	}
}

// readSource reads one schema file from a temporary directory. A test file in
// the project is never parsed.
func readSource(t *testing.T, src string) ([]importer.Model, *importer.Report) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(imports+src), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x_test.go"), []byte("this is not Go"), 0o600); err != nil {
		t.Fatal(err)
	}
	models, rep, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	return models, rep
}

func model(t *testing.T, models []importer.Model, name string) importer.Model {
	t.Helper()
	for _, m := range models {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("no model %s in %+v", name, models)
	return importer.Model{}
}

func field(t *testing.T, m importer.Model, name string) importer.Field {
	t.Helper()
	for _, f := range m.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("model %s has no field %s: %+v", m.Name, name, m.Fields)
	return importer.Field{}
}

func TestReadFixtureModels(t *testing.T) {
	models, rep, err := Read(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range models {
		names = append(names, m.Name)
	}
	if strings.Join(names, " ") != "Card Org Pet Sized User" {
		t.Fatalf("want the fixture models in file and source order, got %v", names)
	}
	if rep.Has(importer.Unsupported) {
		t.Fatalf("the fixture must import cleanly: %s", rep)
	}
	card, org, pet, sized, user := models[0], models[1], models[2], models[3], models[4]
	if card.Table != "cards" || org.Table != "orgs" || pet.Table != "pets" || user.Table != "app_users" || sized.Table != (naming.Ent{}).TableName("Sized") {
		t.Fatalf("tables: %q %q %q %q %q", card.Table, org.Table, pet.Table, sized.Table, user.Table)
	}

	if id := field(t, user, "ID"); !id.PK || !id.Identity || id.Serial || id.SQLType != "bigint" || id.GoType != "int" || id.Nullable {
		t.Fatalf("Ent's key is an identity bigint: %+v", id)
	}
	if e := field(t, user, "Email"); e.SQLType != "varchar" || e.Nullable || !e.Unique || !e.UniqueIndex || e.UniqueName != "app_users_email_key" {
		t.Fatalf("a unique field is a unique index named <table>_<column>_key: %+v", e)
	}
	if n := field(t, user, "Name"); n.GoType != "*string" || !n.Nullable || n.UniqueName != "user_name" || !n.UniqueIndex {
		t.Fatalf("Name is Optional, Nillable and has a unique index: %+v", n)
	}
	if a := field(t, user, "Active"); a.Default != "true" || a.DefaultKind != "db" || a.Nullable || a.GoType != "bool" || a.SQLType != "boolean" || a.IndexName != "app_users_active_idx" || !a.Index {
		t.Fatalf("Active: %+v", a)
	}
	if s := field(t, user, "SeenAt"); !s.Nullable || s.Default != "" || s.SQLType != "timestamptz" {
		t.Fatalf("a mixin field is part of the model; its time.Now default is Go-side: %+v", s)
	}
	if c := field(t, user, "CreatedBy"); c.GoType != "*string" {
		t.Fatalf("mixin field: %+v", c)
	}
	if f := field(t, org, "CreateTime"); f.SQLType != "timestamptz" || f.Nullable || f.GoType != "time.Time" {
		t.Fatalf("mixin.Time: %+v", f)
	}
	if f := field(t, org, "DisplayName"); !f.Nullable || f.SQLType != "varchar" {
		t.Fatalf("MaxLen does not change the PostgreSQL type Ent creates: %+v", f)
	}

	for name, want := range map[string]string{
		"Rank": "integer", "Level": "integer", "Flag": "smallint", "Big": "bigint", "Count": "bigint",
		"Ratio": "double precision", "Weight": "real", "Price": "numeric(10,2)", "Blob": "bytea",
		"Body": "text", "Kind": "varchar", "Label": "varchar",
	} {
		if f := field(t, sized, name); f.SQLType != want {
			t.Fatalf("%s is %s, Ent creates %s", name, f.SQLType, want)
		}
	}
	if f := field(t, sized, "Level"); f.GoType != "uint16" {
		t.Fatalf("Level: %+v", f)
	}
	if f := field(t, sized, "Label"); f.Default != "'it''s'" || f.DefaultKind != "db" {
		t.Fatalf("a string default is quoted and escaped the way Ent writes it: %+v", f)
	}
	if f := field(t, sized, "Kind"); f.Default != "'draft'" {
		t.Fatalf("enum default: %+v", f)
	}

	// foreign keys
	if len(user.ForeignKeys) != 1 {
		t.Fatalf("User holds the key of its Org: %+v", user.ForeignKeys)
	}
	fk := user.ForeignKeys[0]
	if fk.Name != "app_users_orgs_users" || fk.Field != "Org" || fk.RefModel != "Org" || fk.FKField != "OrgUsers" || fk.Columns[0] != "org_users" ||
		fk.RefTable != "orgs" || fk.RefColumns[0] != "id" || fk.OnDelete != "CASCADE" {
		t.Fatalf("the key column is <owner>_<edge>, the action comes from edge.To: %+v", fk)
	}
	if f := field(t, user, "OrgUsers"); f.GoType != "int" || f.Nullable || f.SQLType != "bigint" {
		t.Fatalf("a required edge makes the key NOT NULL: %+v", f)
	}
	if len(pet.ForeignKeys) != 2 {
		t.Fatalf("Pet holds two keys: %+v", pet.ForeignKeys)
	}
	var owner, home importer.ForeignKey
	for _, k := range pet.ForeignKeys {
		if k.Field == "Owner" {
			owner = k
		} else {
			home = k
		}
	}
	if owner.FKField != "OwnerID" || owner.Columns[0] != "owner_id" || owner.OnDelete != "SET NULL" || owner.RefTable != "app_users" || owner.Name != "pets_app_users_pets" {
		t.Fatalf("an edge field is the key column and an optional key sets NULL: %+v", owner)
	}
	if f := field(t, pet, "OwnerID"); f.GoType != "*int" || !f.Nullable {
		t.Fatalf("the edge field is not declared twice: %+v", f)
	}
	if home.Field != "Home" || home.Columns[0] != "home_org" || home.Name != "pets_home_fk" || home.FKField != "HomeOrg" {
		t.Fatalf("StorageKey names the column and the constraint: %+v", home)
	}
	if len(card.ForeignKeys) != 1 || card.ForeignKeys[0].Columns[0] != "user_card" || card.ForeignKeys[0].Name != "cards_app_users_card" || card.ForeignKeys[0].OnDelete != "" {
		t.Fatalf("a one-to-one key lives on the side of edge.From: %+v", card.ForeignKeys)
	}
	if f := field(t, card, "UserCard"); !f.UniqueIndex || f.UniqueName != "cards_user_card_key" || f.Nullable {
		t.Fatalf("a one-to-one key is unique: %+v", f)
	}

	// the report
	if got := rep.Of(importer.ZeroValueHandling); len(got) != 9 {
		t.Fatalf("Active, Score, Kind and Label have a literal default; DisplayName, SeenAt, Count, Blob and Body are nullable without a pointer: %s", rep)
	}
	if got := rep.Of(importer.GoDefault); len(got) != 4 {
		t.Fatalf("create_time and update_time of mixin.Time, SeenAt and JoinedAt: %s", rep)
	}
	if got := rep.Of(importer.ImplicitSave); len(got) != 7 {
		t.Fatalf("every edge is reported once: %s", rep)
	}
	if got := rep.Of(importer.DroppedIndex); len(got) != 1 || got[0].Model != "User" {
		t.Fatalf("one composite index: %s", rep)
	}
	if got := rep.Of(importer.Naming); len(got) != 4 {
		t.Fatalf("four foreign key names: %s", rep)
	}
	if got := rep.Of(importer.GoSideRule); len(got) != 5 {
		t.Fatalf("create_time, MaxLen, Immutable, NotEmpty and the enum values: %s", rep)
	}
	for _, c := range rep.Changes {
		if !sourcePosition.MatchString(c.Source) {
			t.Fatalf("every change carries file:line:col, got %q", c.Source)
		}
	}
}

func TestReadUnsupportedBuilderIsDiagnosed(t *testing.T) {
	dir := t.TempDir()
	src := "package schema\n\nimport (\n\t\"entgo.io/ent\"\n\t\"entgo.io/ent/schema/field\"\n)\n\ntype X struct{ ent.Schema }\n\nfunc (X) Fields() []ent.Field {\n\treturn []ent.Field{field.String(\"x\").Foo()}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	_, rep, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Has("unsupported") {
		t.Fatalf("unknown builder method must be reported as unsupported: %s", rep)
	}
	got := rep.Of(importer.Unsupported)[0]
	line := strings.Split(src, "\n")[10]
	if want := fmt.Sprintf("x.go:11:%d", strings.Index(line, "Foo")+1); got.Source != want || got.Model != "X" || got.Field != "X" {
		t.Fatalf("the report must point at the method: %+v", got)
	}
}

func TestReadReportsWhatItCannotImport(t *testing.T) {
	models, rep := readSource(t, `
type A struct{ ent.Schema }

func (A) Fields() []ent.Field {
	return []ent.Field{
		field.JSON("tags", []string{}),
		field.UUID("uid", uuid.UUID{}),
		field.String("custom").GoType(Foo("")),
		field.Int("id"),
		field.String("dyn").Default(somefn()),
		field.String("keep"),
	}
}

func (A) Edges() []ent.Edge {
	return []ent.Edge{edge.To("others", B.Type)}
}

type B struct{ ent.Schema }

func (B) Edges() []ent.Edge {
	return []ent.Edge{edge.From("as", A.Type).Ref("others")}
}

type C struct{ ent.Schema }

func (C) Fields() []ent.Field {
	fs := []ent.Field{field.String("hidden")}
	return fs
}
`)
	if got := rep.Of(importer.Unsupported); len(got) != 7 {
		t.Fatalf("JSON, UUID, GoType, a custom id, a dynamic default, the many-to-many edge and a Fields body that is not a literal: %s", rep)
	}
	if f := field(t, model(t, models, "A"), "Keep"); f.SQLType != "varchar" {
		t.Fatalf("the supported field next to unsupported ones is still read: %+v", f)
	}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: t.TempDir()})
	err := importer.Emit(models, rep, w, filepath.Join(t.TempDir(), "models"), "models")
	if !errors.Is(err, importer.ErrUnsupported) {
		t.Fatalf("unsupported changes must block Emit, got %v", err)
	}
}

func TestReadRendersLiteralDefaults(t *testing.T) {
	models, rep := readSource(t, `
type D struct{ ent.Schema }

func (D) Fields() []ent.Field {
	return []ent.Field{
		field.Bool("b").Default(false),
		field.Int("i").Default(-5),
		field.Uint8("u").Default(7),
		field.Float("f").Default(1.50),
		field.Float32("g").Default(2),
		field.String("s").Default("it's"),
		field.Text("t").Optional().Nillable().Default("x"),
		field.Enum("e").Values("a", "b").Default("a"),
		field.String("expr").Annotations(entsql.DefaultExpr("lower('A')")),
		field.Time("at").Default(time.Now),
	}
}
`)
	d := model(t, models, "D")
	for name, want := range map[string]string{"B": "false", "I": "-5", "U": "7", "F": "1.5", "G": "2", "S": "'it''s'", "T": "'x'", "E": "'a'", "Expr": "(lower('A'))", "At": ""} {
		if got := field(t, d, name).Default; got != want {
			t.Errorf("default of %s = %q, want %q", name, got, want)
		}
	}
	if rep.Has(importer.Unsupported) {
		t.Fatalf("all defaults are literals: %s", rep)
	}
	if got := rep.Of(importer.ZeroValueHandling); len(got) != 7 {
		t.Fatalf("every non-pointer field with a literal default, and no other: %s", rep)
	}
	if got := rep.Of(importer.GoDefault); len(got) != 1 || got[0].Field != "At" {
		t.Fatalf("Default(time.Now) is applied in Go: %s", rep)
	}
}

func TestReadResolvesEdgesWithoutBackReferences(t *testing.T) {
	models, rep := readSource(t, `
type Parent struct{ ent.Schema }

func (Parent) Edges() []ent.Edge {
	return []ent.Edge{edge.To("kids", Kid.Type)}
}

type Kid struct{ ent.Schema }

type Node struct{ ent.Schema }

func (Node) Edges() []ent.Edge {
	return []ent.Edge{edge.To("next", Node.Type).Unique()}
}

type Task struct{ ent.Schema }

func (Task) Edges() []ent.Edge {
	return []ent.Edge{edge.To("owner", Parent.Type).Unique().Required()}
}
`)
	if rep.Has(importer.Unsupported) {
		t.Fatalf("%s", rep)
	}
	kid := model(t, models, "Kid")
	if len(kid.ForeignKeys) != 1 {
		t.Fatalf("a non-unique edge.To without a back reference puts the key on the target: %+v", kid.ForeignKeys)
	}
	if fk := kid.ForeignKeys[0]; fk.Field != "Parent" || fk.FKField != "ParentKids" || fk.Columns[0] != "parent_kids" || fk.RefTable != "parents" || fk.OnDelete != "SET NULL" || fk.Name != "kids_parents_kids" {
		t.Fatalf("kid key: %+v", fk)
	}
	if f := field(t, kid, "ParentKids"); f.GoType != "*int" || !f.Nullable {
		t.Fatalf("kid key column: %+v", f)
	}
	node := model(t, models, "Node")
	if fk := node.ForeignKeys[0]; fk.Field != "Next" || fk.Columns[0] != "node_next" || fk.RefTable != "nodes" || fk.OnDelete != "SET NULL" {
		t.Fatalf("a self-referencing unique edge: %+v", fk)
	}
	if f := field(t, node, "NodeNext"); !f.UniqueIndex || f.UniqueName != "nodes_node_next_key" || !f.Nullable {
		t.Fatalf("a self-referencing one-to-one key is unique and nullable: %+v", f)
	}
	task := model(t, models, "Task")
	if fk := task.ForeignKeys[0]; fk.Field != "Owner" || fk.Columns[0] != "task_owner" || fk.OnDelete != "" {
		t.Fatalf("a required unique edge: %+v", fk)
	}
	if f := field(t, task, "TaskOwner"); f.GoType != "int" || f.Nullable {
		t.Fatalf("a required key is NOT NULL: %+v", f)
	}
	var withoutEdge int
	for _, c := range rep.Of(importer.Naming) {
		if strings.Contains(c.Message, "without an edge there") {
			withoutEdge++
		}
	}
	if withoutEdge != 1 {
		t.Fatalf("the key on Kid has no edge on Kid and must say so: %s", rep)
	}
}

func TestReadRefusesAnnotationsAndFieldsItWouldIgnore(t *testing.T) {
	_, rep := readSource(t, `
type Org struct{ ent.Schema }

func (Org) Edges() []ent.Edge {
	return []ent.Edge{edge.To("users", User.Type)}
}

type User struct{ ent.Schema }

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "people", Charset: "utf8"}, entgql.QueryField()}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("a").Nillable(),
		field.Int("org_id").Optional().Unique(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{edge.From("org", Org.Type).Ref("users").Unique().Field("org_id").Annotations(entsql.OnDelete(entsql.Cascade))}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{index.Fields("a", "missing")}
}
`)
	got := rep.Of(importer.Unsupported)
	var msgs []string
	for _, c := range got {
		msgs = append(msgs, c.Message)
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{"Charset", "Nillable without Optional", "OnDelete from the edge.To", "Unique on the edge field", "missing"} {
		if !strings.Contains(all, want) {
			t.Errorf("no unsupported change mentions %q:\n%s", want, rep)
		}
	}
}

func TestReadAppliesMixinsConfigAndIndexes(t *testing.T) {
	models, rep := readSource(t, `
type Tagged struct{ mixin.Schema }

func (Tagged) Fields() []ent.Field {
	return []ent.Field{field.String("tag")}
}

type T struct{ ent.Schema }

func (T) Config() ent.Config {
	return ent.Config{Table: "cfg"}
}

func (T) Mixin() []ent.Mixin {
	return []ent.Mixin{Tagged{}, mixin.CreateTime{}}
}

func (T) Hooks() []ent.Hook {
	return nil
}

func (T) Fields() []ent.Field {
	return []ent.Field{field.String("a"), field.String("b")}
}

func (T) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("a").Unique(),
		index.Fields("a"),
		index.Fields("b").StorageKey("b_idx"),
	}
}

type U struct{ ent.Schema }

func (U) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "u_table"}}
}

func (U) Config() ent.Config {
	return ent.Config{Table: "ignored"}
}
`)
	if rep.Has(importer.Unsupported) {
		t.Fatalf("%s", rep)
	}
	tm := model(t, models, "T")
	if tm.Table != "cfg" {
		t.Fatalf("Config Table: %q", tm.Table)
	}
	field(t, tm, "Tag")
	if c := field(t, tm, "CreateTime"); c.SQLType != "timestamptz" || c.Nullable {
		t.Fatalf("mixin.CreateTime: %+v", c)
	}
	if a := field(t, tm, "A"); !a.UniqueIndex || a.UniqueName != "t_a" || a.Index {
		t.Fatalf("index.Fields(a).Unique(): %+v", a)
	}
	if b := field(t, tm, "B"); !b.Index || b.IndexName != "b_idx" {
		t.Fatalf("StorageKey names the index: %+v", b)
	}
	if got := rep.Of(importer.DroppedIndex); len(got) != 1 || got[0].Field != "A" {
		t.Fatalf("a second index on one field is reported and dropped: %s", rep)
	}
	if got := rep.Of(importer.GoSideRule); len(got) != 2 {
		t.Fatalf("the hook and the immutable create_time: %s", rep)
	}
	if u := model(t, models, "U"); u.Table != "u_table" {
		t.Fatalf("an entsql Table annotation wins over Config: %q", u.Table)
	}
}

func TestReadIsDeterministic(t *testing.T) {
	_, first, err := Read(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_, again, err := Read(fixtureDir)
		if err != nil {
			t.Fatal(err)
		}
		if first.String() != again.String() || len(first.Changes) != len(again.Changes) {
			t.Fatalf("two reads of the same directory differ:\n%s\n%s", first, again)
		}
		for j := range first.Changes {
			if first.Changes[j] != again.Changes[j] {
				t.Fatalf("change %d moved between reads: %+v vs %+v", j, first.Changes[j], again.Changes[j])
			}
		}
	}
}

func TestReadEntityAnnotationsAreNotDroppedSilently(t *testing.T) {
	models, rep := readSource(t, `
type A struct{ ent.Schema }

func (A) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Table("a_tab"), entgql.QueryField()}
}

type B struct{ ent.Schema }

func (B) Annotations() []schema.Annotation {
	return []schema.Annotation{&entsql.Annotation{Table: "b_tab"}}
}

type C struct{ ent.Schema }

func (C) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Skip(), entsql.WithComments(true), entsql.Checks(nil), mine, helper()}
}

type D struct{ ent.Schema }

func (D) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.SchemaTable("d_tab")}
}
`)
	if got := model(t, models, "A").Table; got != "a_tab" {
		t.Errorf("entsql.Table call: %q", got)
	}
	if got := model(t, models, "B").Table; got != "b_tab" {
		t.Errorf("&entsql.Annotation literal: %q", got)
	}
	if got := model(t, models, "D").Table; got != "d_tab" {
		t.Errorf("entsql.SchemaTable call: %q", got)
	}
	// Skip, WithComments and Checks are entsql; mine and helper() cannot be proven to be another package's
	if got := rep.Of(importer.Unsupported); len(got) != 5 {
		t.Fatalf("every annotation that might change the schema is reported, the entgql one is not: %s", rep)
	}
}

func TestReadFieldAndEdgeAnnotationsBehindPointersAndVariables(t *testing.T) {
	models, rep := readSource(t, `
type Org struct{ ent.Schema }

func (Org) Edges() []ent.Edge {
	return []ent.Edge{edge.To("users", User.Type).Annotations(&entsql.Annotation{OnDelete: entsql.Cascade})}
}

type User struct{ ent.Schema }

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Time("at").Annotations(entsql.Default("now()")),
		field.String("s").Annotations(entsql.Default("x")),
		field.String("e").Annotations(&entsql.Annotation{DefaultExpr: "(lower('A'))"}),
		field.String("v").Annotations(shared),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{edge.From("org", Org.Type).Ref("users").Unique()}
}
`)
	user := model(t, models, "User")
	if len(user.ForeignKeys) != 1 || user.ForeignKeys[0].OnDelete != "CASCADE" {
		t.Fatalf("OnDelete behind & must not be lost: %+v", user.ForeignKeys)
	}
	for name, want := range map[string]string{"At": "now()", "S": "'x'", "E": "(lower('A'))"} {
		if got := field(t, user, name).Default; got != want {
			t.Errorf("default of %s = %q, want %q (Ent quotes a string default except on time columns and wraps an expression in parentheses)", name, got, want)
		}
	}
	if got := rep.Of(importer.Unsupported); len(got) != 1 || got[0].Field != "V" {
		t.Fatalf("a variable annotation is reported: %s", rep)
	}
}

func TestReadChecksEveryMixinContribution(t *testing.T) {
	models, rep := readSource(t, `
type Guarded struct{ mixin.Schema }

func (Guarded) Fields() []ent.Field {
	return []ent.Field{field.String("g")}
}

func (Guarded) Hooks() []ent.Hook { return nil }

func (Guarded) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Skip(), entsql.Table("from_mixin")}
}

type Outer struct{ mixin.Schema }

func (Outer) Mixin() []ent.Mixin { return []ent.Mixin{Guarded{}} }

type T struct{ ent.Schema }

func (T) Mixin() []ent.Mixin { return []ent.Mixin{Guarded{}, Outer{}} }
`)
	if got := rep.Of(importer.GoSideRule); len(got) != 1 || !strings.Contains(got[0].Message, "Guarded") {
		t.Fatalf("the hook of the mixin: %s", rep)
	}
	var skip, nested int
	for _, c := range rep.Of(importer.Unsupported) {
		if strings.Contains(c.Message, "Skip") {
			skip++
		}
		if strings.Contains(c.Message, "nested mixins") {
			nested++
		}
	}
	if skip != 1 || nested != 1 {
		t.Fatalf("Skip in a mixin and a nested Mixin() must be reported: %s", rep)
	}
	if got := model(t, models, "T").Table; got != "from_mixin" {
		t.Fatalf("a table named by a mixin annotation applies to the entity: %q", got)
	}
	field(t, model(t, models, "T"), "G")
}

func TestReadReportsSchemaTypesThatEmbedSchemaTypes(t *testing.T) {
	models, rep := readSource(t, `
type Base struct{ ent.Schema }

func (Base) Fields() []ent.Field {
	return []ent.Field{field.String("b")}
}

type Derived struct{ Base }

type Deeper struct{ *Derived }
`)
	got := rep.Of(importer.Unsupported)
	if len(got) != 2 || got[0].Model != "Derived" || got[1].Model != "Deeper" {
		t.Fatalf("a struct embedding a schema type, directly or not, is reported: %s", rep)
	}
	if len(models) != 1 || models[0].Name != "Base" {
		t.Fatalf("only Base is an entity: %+v", models)
	}
}

func TestReadMapsHugeMaxLenToText(t *testing.T) {
	models, rep := readSource(t, `
const (
	nameLen = 100
	hugeLen = 1 << 25
	sumLen  = hugeLen + nameLen
)

type S struct{ ent.Schema }

func (S) Fields() []ent.Field {
	return []ent.Field{
		field.String("small").MaxLen(100),
		field.String("huge").MaxLen(20000000),
		field.String("typed").MaxLen(20000000).SchemaType(map[string]string{dialect.Postgres: "citext"}),
		field.String("shifted").MaxLen(1 << 20),
		field.String("shiftedhuge").MaxLen(1 << 25),
		field.String("named").MaxLen(nameLen),
		field.String("namedhuge").MaxLen(hugeLen),
		field.String("sum").MaxLen((hugeLen + 1) * 2 - 2),
		field.String("chained").MaxLen(sumLen),
		field.String("dyn").MaxLen(limit),
		field.String("call").MaxLen(compute(3)),
	}
}
`)
	s := model(t, models, "S")
	for name, want := range map[string]string{
		"Small": "varchar", "Huge": "text", "Typed": "citext", "Shifted": "varchar", "Shiftedhuge": "text",
		"Named": "varchar", "Namedhuge": "text", "Sum": "text", "Chained": "text", "Dyn": "varchar", "Call": "varchar",
	} {
		if got := field(t, s, name).SQLType; got != want {
			t.Errorf("%s is %s, want %s (Ent creates text above 10485760)", name, got, want)
		}
	}
	if rep.Has(importer.Unsupported) {
		t.Fatalf("an unreadable MaxLen is reported, not unsupported: %s", rep)
	}
	got := rep.Of(importer.RenamedType)
	if len(got) != 2 || got[0].Field != "Dyn" || got[1].Field != "Call" {
		t.Fatalf("the two sizes that cannot be folded are reported: %s", rep)
	}
}

func TestReadReportsNullableFieldsWithoutPointers(t *testing.T) {
	_, rep := readSource(t, `
type N struct{ ent.Schema }

func (N) Fields() []ent.Field {
	return []ent.Field{
		field.String("plain").Optional(),
		field.String("ptr").Optional().Nillable(),
		field.Int("req"),
	}
}
`)
	got := rep.Of(importer.ZeroValueHandling)
	if len(got) != 1 || got[0].Field != "Plain" {
		t.Fatalf("only the optional field without a pointer is reported: %s", rep)
	}
}
