// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Catches: introspect.only being ignored, so a table outside the list still
// gets a model and a place in the init snapshot.
func TestOnlyLimitsModelsAndSnapshot(t *testing.T) {
	res, files := generate(t, sampleSchema(), Config{Only: []string{"users"}})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	if len(res.Files) != 1 || files["users"] == "" || files["orders"] != "" {
		t.Fatalf("files = %d, want only the users model", len(res.Files))
	}
	if len(res.Snapshot.Tables) != 1 || res.Snapshot.Tables[0].Name != "users" {
		t.Fatalf("snapshot tables = %+v, want only users", res.Snapshot.Tables)
	}
}

// Catches: skip not applying within only, or only widening what skip removed.
func TestOnlyAndSkipTogether(t *testing.T) {
	res, files := generate(t, sampleSchema(), Config{Only: []string{"o*", "u*"}, Skip: []string{"orders"}})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	if len(res.Files) != 1 || files["users"] == "" {
		t.Fatalf("files = %d, want only users (orders is skipped inside only)", len(res.Files))
	}
}

// Catches: the triggers of a table outside only staying in the models and the
// snapshot, where they would name a table with no model.
func TestOnlyDropsObjectsOfOtherTables(t *testing.T) {
	s := sampleSchema()
	s.Functions = []schema.Function{{Name: "touch", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: "BEGIN RETURN NEW; END;"}}
	s.Triggers = []schema.Trigger{{Table: "orders", Name: "orders_touch", Timing: "BEFORE", Events: []string{"UPDATE"}, ForEachRow: true, Function: "touch()"}}
	res, files := generate(t, s, Config{Only: []string{"users"}})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	if strings.Contains(files["users"], "orders_touch") || len(res.Snapshot.Triggers) != 0 {
		t.Fatalf("the trigger of an unmodeled table must be dropped: %v", res.Snapshot.Triggers)
	}
}

// Catches: a malformed or misspelled option passing validation and failing
// later, or silently doing nothing.
func TestIntrospectScopeValidation(t *testing.T) {
	for name, c := range map[string]Config{
		"only glob":    {Only: []string{"["}},
		"functions":    {Functions: "some"},
		"dbdefault":    {DBDefault: "yes"},
		"functions ok": {Functions: "all"},
		"dbdefault ok": {DBDefault: "all"},
	} {
		err := c.Validate()
		if want := !strings.HasSuffix(name, " ok"); (err != nil) != want {
			t.Errorf("%s: Validate = %v, want error %v", name, err, want)
		}
	}
}

// scopeSchema has three modeled tables and functions with different users.
func scopeSchema() *schema.Schema {
	fn := func(name, ret, body string) schema.Function {
		return schema.Function{Name: name, Returns: ret, Language: "plpgsql", Volatility: "VOLATILE", Body: body}
	}
	idCol := schema.Column{Name: "id", Type: "bigint", Identity: "by_default"}
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Tables: []schema.Table{
			{Name: "alpha", Columns: []schema.Column{idCol, {Name: "owner", Type: "text"}}, PrimaryKey: []string{"id"}},
			{Name: "beta", Columns: []schema.Column{idCol, {Name: "n", Type: "integer"}}, PrimaryKey: []string{"id"}},
			{
				Name: "gamma", Columns: []schema.Column{idCol, {Name: "code", Type: "text", Default: "next_code()"}}, PrimaryKey: []string{"id"},
				Checks: []schema.Check{{Name: "gamma_code_check", Expr: "(char_length(code) > 0)"}},
			},
		},
		Functions: []schema.Function{
			fn("touch", "trigger", "BEGIN PERFORM touch_helper(); RETURN NEW; END;"),
			fn("touch_helper", "void", "BEGIN NULL; END;"),
			fn("next_code", "text", "BEGIN RETURN 'x'; END;"),
			fn("can_read", "boolean", "BEGIN RETURN true; END;"),
			fn("unrelated_fn", "void", "BEGIN NULL; END;"),
		},
		Triggers: []schema.Trigger{{Table: "beta", Name: "beta_touch", Timing: "BEFORE", Events: []string{"UPDATE"}, ForEachRow: true, Function: "touch()"}},
		Policies: []schema.Policy{{Table: "alpha", Name: "alpha_read", Command: "SELECT", Permissive: true, Using: "can_read(owner)"}},
	}
}

// Catches: every schema function landing on the alphabetically first model
// (issue 74): a function no modeled table uses must not be written, and one a
// table uses must sit on that table's model, with the functions it calls.
func TestFunctionsOnlyThoseModeledTablesUse(t *testing.T) {
	res, files := generate(t, scopeSchema(), Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	var all strings.Builder
	for _, src := range files {
		all.WriteString(src)
	}
	if strings.Contains(all.String(), "unrelated_fn") {
		t.Error("a function no modeled table uses was written")
	}
	mustContain(t, "beta", files["beta"], `schema.Function{Name: "touch"`, `schema.Function{Name: "touch_helper"`, "beta_touch")
	mustContain(t, "gamma", files["gamma"], `schema.Function{Name: "next_code"`)
	mustContain(t, "alpha", files["alpha"], `schema.Function{Name: "can_read"`, "alpha_read")
	for _, fn := range []string{"touch", "touch_helper", "next_code"} {
		if strings.Contains(files["alpha"], `schema.Function{Name: "`+fn+`"`) {
			t.Errorf("alpha, the first model, carries %s", fn)
		}
	}
	names := map[string]bool{}
	for _, f := range res.Snapshot.Functions {
		names[f.Name] = true
	}
	if names["unrelated_fn"] || !names["touch"] || !names["touch_helper"] || !names["next_code"] || !names["can_read"] {
		t.Errorf("snapshot functions = %v", names)
	}
}

// Catches: introspect.functions: all not restoring the old behaviour of every
// function on the first model.
func TestFunctionsAllRestoresOldBehaviour(t *testing.T) {
	res, files := generate(t, scopeSchema(), Config{Functions: "all"})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	for _, fn := range []string{"touch", "touch_helper", "next_code", "can_read", "unrelated_fn"} {
		mustContain(t, "alpha", files["alpha"], `schema.Function{Name: "`+fn+`"`)
	}
	if strings.Contains(files["beta"], "schema.Function{") || strings.Contains(files["gamma"], "schema.Function{") {
		t.Error("with functions: all every function belongs on the first model")
	}
}

// Catches: the generated file of a model changing when something unrelated
// changes in the database (issue 74: the header hash changed on every
// migration).
func TestModelFilesStableAgainstUnrelatedChanges(t *testing.T) {
	_, before := generate(t, scopeSchema(), Config{})
	s := scopeSchema()
	s.Functions = append(s.Functions, schema.Function{Name: "added_later", Returns: "void", Language: "plpgsql", Volatility: "VOLATILE", Body: "BEGIN NULL; END;"})
	s.Tables = append(s.Tables, schema.Table{Name: "zeta", Columns: []schema.Column{{Name: "id", Type: "bigint", Identity: "by_default"}}, PrimaryKey: []string{"id"}})
	s.Triggers = append(s.Triggers, schema.Trigger{Table: "zeta", Name: "zeta_touch", Timing: "BEFORE", Events: []string{"UPDATE"}, ForEachRow: true, Function: "added_later()"})
	_, after := generate(t, s, Config{})
	for _, table := range []string{"alpha", "beta", "gamma"} {
		if before[table] != after[table] {
			t.Errorf("the model of %s changed when an unrelated function and table were added:\n%s\n---\n%s", table, before[table], after[table])
		}
	}
}

// Catches: dbdefault: all missing the constant default (version bigint NOT
// NULL DEFAULT 1), which Insert.Model then writes as 0 (issue 77).
func TestDBDefaultAllTagsConstantDefaults(t *testing.T) {
	s := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: dialect.Postgres, Tables: []schema.Table{{
		Name: "overrides",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "version", Type: "bigint", Default: "1"},
			{Name: "state", Type: "text", Default: "'open'::text"},
			{Name: "note", Type: "text", Nullable: true, Default: "'x'::text"},
			{Name: "plain", Type: "text"},
			{Name: "active", Type: "boolean", Default: "true"},
			{Name: "off", Type: "boolean", Default: "false"},
			{Name: "count", Type: "integer", Default: "0"},
		},
		PrimaryKey: []string{"id"},
	}}}
	_, def := generate(t, s, Config{})
	if strings.Contains(def["overrides"], "dbdefault") {
		t.Error("by default a constant default must not get dbdefault")
	}
	_, all := generate(t, s, Config{DBDefault: "all"})
	src := all["overrides"]
	mustContain(t, "overrides", src, "`momo:\"default:1,dbdefault\"`", "`momo:\"default:'open'::text,dbdefault\"`")
	// A NOT NULL bool or number whose constant default is true or non-zero is
	// a pointer, so an explicit false or 0 is not replaced by the default; a
	// zero default and a text default keep the plain type (issue 90).
	mustMatch(t, "overrides", src,
		`Version\s+\*int64\s+`+"`momo:\"default:1,dbdefault\"`",
		`Active\s+\*bool\s+`+"`momo:\"default:true,dbdefault\"`",
		`Off\s+bool\s+`+"`momo:\"default:false,dbdefault\"`",
		`Count\s+int32\s+`+"`momo:\"default:0,dbdefault\"`",
		`State\s+string\s+`,
	)
	for _, line := range strings.Split(src, "\n") {
		if (strings.Contains(line, "Note ") || strings.Contains(line, "Plain ")) && strings.Contains(line, "dbdefault") {
			t.Errorf("a nullable or default-less column got dbdefault: %s", line)
		}
	}
}

// Catches: only and skip matching the bare table name while migrate also
// matches schema.name, so `only: ["app.*"]` wrote no model for a table the
// migrate side managed.
func TestOnlyAndSkipMatchSchemaQualifiedNames(t *testing.T) {
	col := []schema.Column{{Name: "id", Type: "bigint", Identity: "by_default"}}
	s := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: dialect.Postgres, Tables: []schema.Table{
		{Schema: "app", Name: "widgets", Columns: col, PrimaryKey: []string{"id"}},
		{Schema: "app", Name: "gadgets", Columns: col, PrimaryKey: []string{"id"}},
		{Name: "legacy", Columns: col, PrimaryKey: []string{"id"}},
	}}
	res, files := generate(t, s, Config{Only: []string{"app.*"}})
	if res.Failed() || len(res.Files) != 2 || files["legacy"] != "" {
		t.Fatalf("only app.*: %d files, legacy model %v, findings %v", len(res.Files), files["legacy"] != "", res.Findings)
	}
	res, files = generate(t, s, Config{Only: []string{"app.*"}, Skip: []string{"app.gadgets"}})
	if res.Failed() || len(res.Files) != 1 || files["widgets"] == "" {
		t.Fatalf("only app.*, skip app.gadgets: %d files, findings %v", len(res.Files), res.Findings)
	}
}

// Catches: an empty entry in introspect.only passing validation (a YAML `- `
// is an empty string that matches no table, so no model would be written).
func TestOnlyRejectsAnEmptyGlob(t *testing.T) {
	for _, c := range []Config{{Only: []string{""}}, {Only: []string{"users", " "}}} {
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "introspect.only[") {
			t.Errorf("only %q: err = %v, want an introspect.only error", c.Only, err)
		}
	}
}

// Catches: a foreign key into a table outside only being written as if the
// target had a model, which the models cannot declare.
func TestOnlyDropsForeignKeyToUnmodeledTableWithWarning(t *testing.T) {
	res, files := generate(t, sampleSchema(), Config{Only: []string{"orders"}})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	if strings.Contains(files["orders"], "belongs_to") {
		t.Error("the relation to the unmodeled users table was written")
	}
	var warned bool
	for _, f := range res.Findings {
		warned = warned || (f.Severity == SevWarn && strings.Contains(f.Msg, "users, which has no model"))
	}
	if !warned {
		t.Errorf("the dropped foreign key must be reported: %v", res.Findings)
	}
}

// dependencySchema is one table whose default, check and function arguments
// reach functions in ways a plain call scan can miss.
func dependencySchema() *schema.Schema {
	fn := func(name, args, body string) schema.Function {
		return schema.Function{Name: name, Args: args, Returns: "integer", Language: "plpgsql", Volatility: "VOLATILE", Body: body}
	}
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Tables: []schema.Table{{
			Name: "t",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "by_default"},
				{Name: "n", Type: "integer", Default: "outer_fn()"},
				{Name: "d", Type: "integer", Default: "dyn_caller()"},
				{Name: "ref", Type: "bigint"},
			},
			PrimaryKey: []string{"id"},
			Checks: []schema.Check{
				{Name: "t_ref_check", Expr: "(ref = ('reg_fn'::regproc)::oid)"},
				{Name: "t_ref2_check", Expr: "(ref = ('public.reg_two(integer)'::regprocedure)::oid)"},
			},
		}},
		Functions: []schema.Function{
			fn("outer_fn", "x integer DEFAULT arg_default_fn()", "BEGIN RETURN 1; END;"),
			fn("arg_default_fn", "", "BEGIN RETURN 1; END;"),
			fn("reg_fn", "", "BEGIN RETURN 1; END;"),
			fn("reg_two", "integer", "BEGIN RETURN 1; END;"),
			fn("dyn_caller", "", "BEGIN EXECUTE 'SELECT ' || 'dyn_' || 'target()'; RETURN 1; END;"),
			fn("dyn_target", "", "BEGIN RETURN 1; END;"),
		},
	}
}

// Catches: a function reached only through another function's argument default
// or through a regproc or regprocedure literal being left out, so the models
// fail to apply on a fresh database.
func TestFunctionsFollowArgumentDefaultsAndRegprocLiterals(t *testing.T) {
	res, files := generate(t, dependencySchema(), Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	for _, fn := range []string{"outer_fn", "arg_default_fn", "reg_fn", "reg_two", "dyn_caller"} {
		mustContain(t, "t", files["t"], `schema.Function{Name: "`+fn+`"`)
	}
}

// Documents the limit: SQL built at run time (EXECUTE with a name put together
// from pieces) cannot be followed, so the function it reaches is not written
// by default; introspect.functions: all writes it.
func TestFunctionsCannotFollowDynamicSQL(t *testing.T) {
	_, files := generate(t, dependencySchema(), Config{})
	if strings.Contains(files["t"], `schema.Function{Name: "dyn_target"`) {
		t.Error("dyn_target is reached only by EXECUTE with a built name and cannot be detected")
	}
	_, all := generate(t, dependencySchema(), Config{Functions: "all"})
	mustContain(t, "t", all["t"], `schema.Function{Name: "dyn_target"`)
}

// Catches: regproc references the parser misses: a quoted name that holds a
// dot, a pg_catalog-qualified cast and to_regproc('name'). A missed function
// breaks apply on a fresh database.
func TestFunctionsFollowQuotedAndQualifiedRegprocReferences(t *testing.T) {
	fn := func(name string) schema.Function {
		return schema.Function{Name: name, Returns: "integer", Language: "plpgsql", Volatility: "VOLATILE", Body: "BEGIN RETURN 1; END;"}
	}
	s := &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Tables: []schema.Table{{
			Name:       "t",
			Columns:    []schema.Column{{Name: "id", Type: "bigint", Identity: "by_default"}, {Name: "ref", Type: "bigint"}},
			PrimaryKey: []string{"id"},
			Checks: []schema.Check{
				{Name: "c1", Expr: `(ref = ('"odd.fn"'::regproc)::oid)`},
				{Name: "c2", Expr: `(ref = ('cat_fn'::pg_catalog.regproc)::oid)`},
				{Name: "c3", Expr: `(ref = (to_regproc('toreg_fn'))::oid)`},
				{Name: "c4", Expr: `(ref = ('public."quoted_fn"(integer)'::regprocedure)::oid)`},
			},
		}},
		Functions: []schema.Function{fn("odd.fn"), fn("cat_fn"), fn("toreg_fn"), fn("quoted_fn"), fn("never_used")},
	}
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	for _, name := range []string{"odd.fn", "cat_fn", "toreg_fn", "quoted_fn"} {
		mustContain(t, "t", files["t"], `schema.Function{Name: "`+name+`"`)
	}
	if strings.Contains(files["t"], `Name: "never_used"`) {
		t.Error("a function nothing refers to was written")
	}
}
