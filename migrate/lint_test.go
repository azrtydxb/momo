// SPDX-License-Identifier: Apache-2.0

package migrate

import "testing"

func TestMigrateLint(t *testing.T) {
	parent := map[string]bool{"docs": true}
	cases := []struct {
		name string
		sql  string
		rule string
	}{
		{"not null add", "ALTER TABLE docs ADD COLUMN tenant bigint NOT NULL;", RuleNotNullWithoutDefault},
		{"type change", "ALTER TABLE docs ALTER COLUMN title TYPE varchar(10);", RuleColumnTypeChange},
		{"blocking index", "CREATE INDEX docs_title ON docs (title);", RuleBlockingIndex},
		{"fk without not valid", "ALTER TABLE docs ADD CONSTRAINT fk FOREIGN KEY (org) REFERENCES orgs(id);", RuleForeignKeyWithoutNotValid},
		{"check without not valid", "ALTER TABLE docs ADD CONSTRAINT ck CHECK (length(title) > 0);", RuleForeignKeyWithoutNotValid},
		{"drop table", "DROP TABLE docs;", RuleDestructiveWithoutDirective},
		{"truncate", "TRUNCATE docs;", RuleDestructiveWithoutDirective},
		{"drop column", "ALTER TABLE docs DROP COLUMN title;", RuleDestructiveWithoutDirective},
		{"quoted and qualified table", `ALTER TABLE "public"."Docs" ADD COLUMN tenant bigint NOT NULL;`, RuleNotNullWithoutDefault},
	}
	for _, c := range cases {
		tables := parent
		if c.name == "quoted and qualified table" {
			tables = map[string]bool{"public.docs": true}
		}
		got := LintSQL("20260101000000", []Statement{{SQL: c.sql}}, tables, "postgres")
		found := false
		for _, f := range got {
			if f.Rule == c.rule && f.Severity == SeverityError && f.Alternative != "" && f.Statement == 1 && f.Version == "20260101000000" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: rule %s not reported with severity and alternative: %+v", c.name, c.rule, got)
		}
	}
}

// Catches: a rule that fires on safe SQL would make every migration fail lint.
func TestMigrateLintQuietOnSafeSQL(t *testing.T) {
	parent := map[string]bool{"docs": true}
	safe := []string{
		"ALTER TABLE docs ADD COLUMN tenant bigint NOT NULL DEFAULT 0;",
		"ALTER TABLE docs ADD COLUMN tenant bigint;",
		"ALTER TABLE docs ADD COLUMN id2 bigserial NOT NULL;",
		"CREATE INDEX CONCURRENTLY docs_title ON docs (title);",
		"ALTER TABLE docs ADD CONSTRAINT fk FOREIGN KEY (org) REFERENCES orgs(id) NOT VALID;",
		"ALTER TABLE docs VALIDATE CONSTRAINT fk;",
		"ALTER TABLE docs DROP CONSTRAINT fk;",
		"ALTER TABLE docs ALTER COLUMN title DROP NOT NULL;",
		"CREATE TABLE fresh (id int NOT NULL);",
		"CREATE INDEX fresh_id ON fresh (id);",
		"ALTER TABLE fresh ADD COLUMN x int NOT NULL;",
		"-- DROP TABLE docs;\nSELECT 1;",
		"INSERT INTO docs (title) VALUES ('drop table x');",
	}
	for _, sql := range safe {
		if got := LintSQL("20260101000000", []Statement{{SQL: sql}}, parent, "postgres"); len(got) != 0 {
			t.Errorf("%q: unexpected findings %+v", sql, got)
		}
	}
}

func TestMigrateLintSetNotNullIsAWarning(t *testing.T) {
	got := LintSQL("v", []Statement{{SQL: "ALTER TABLE docs ALTER COLUMN title SET NOT NULL;"}}, map[string]bool{"docs": true}, "postgres")
	if len(got) != 1 || got[0].Severity != SeverityWarning || got[0].Rule != RuleNotNullWithoutDefault || got[0].Alternative == "" {
		t.Fatalf("want one warning, got %+v", got)
	}
	if HasErrors(got) {
		t.Fatal("a warning is not an error")
	}
}

func TestMigrateLintSQLiteRebuildAndEnum(t *testing.T) {
	got := LintSQL("v", []Statement{{Hint: "rebuild"}}, nil, "sqlite")
	if len(got) != 1 || got[0].Rule != RuleSQLiteRebuild || got[0].Severity != SeverityWarning || got[0].Alternative == "" {
		t.Fatalf("rebuild: %+v", got)
	}
	enum := []Statement{
		{SQL: "ALTER TYPE status ADD VALUE 'Archived';"},
		{SQL: "UPDATE docs SET status = 'Archived' WHERE old;"},
	}
	got = LintSQL("v", enum, nil, "postgres")
	if len(got) != 1 || got[0].Rule != RuleEnumAddAndUse || got[0].Severity != SeverityError || got[0].Statement != 2 || got[0].Alternative == "" {
		t.Fatalf("enum add and use: %+v", got)
	}
	// Adding the value without using it is fine.
	if got := LintSQL("v", enum[:1], nil, "postgres"); len(got) != 0 {
		t.Fatalf("enum add alone: %+v", got)
	}
	// The PostgreSQL rules do not run on other dialects; the destructive rule does.
	if got := LintSQL("v", []Statement{{SQL: "ALTER TABLE docs ADD COLUMN t int NOT NULL"}}, map[string]bool{"docs": true}, "mysql"); len(got) != 0 {
		t.Fatalf("mysql: %+v", got)
	}
	if got := LintSQL("v", []Statement{{SQL: "DROP TABLE docs"}}, nil, "mysql"); len(got) != 1 {
		t.Fatalf("mysql destructive: %+v", got)
	}
}
