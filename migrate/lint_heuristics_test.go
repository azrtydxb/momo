// SPDX-License-Identifier: Apache-2.0

package migrate

import "testing"

// Catches: the SET NOT NULL warning telling the user to validate a CHECK first
// and then warning again after they did.
func TestMigrateLintNotNullAdviceIsFollowable(t *testing.T) {
	parent := map[string]bool{"docs": true}
	followed := []Statement{
		{SQL: "ALTER TABLE docs ADD CONSTRAINT title_nn CHECK (title IS NOT NULL) NOT VALID;"},
		{SQL: "ALTER TABLE docs VALIDATE CONSTRAINT title_nn;"},
		{SQL: "ALTER TABLE docs ALTER COLUMN title SET NOT NULL;"},
	}
	if got := LintSQL("v", followed, parent, "postgres"); len(got) != 0 {
		t.Fatalf("the advised sequence must be clean: %+v", got)
	}
	// Without the validation the warning stays.
	unvalidated := []Statement{followed[0], followed[2]}
	if got := LintSQL("v", unvalidated, parent, "postgres"); len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("an unvalidated CHECK does not help: %+v", got)
	}
	// A validated CHECK on another column does not help either.
	other := []Statement{followed[0], followed[1], {SQL: "ALTER TABLE docs ALTER COLUMN body SET NOT NULL;"}}
	if got := LintSQL("v", other, parent, "postgres"); len(got) != 1 {
		t.Fatalf("another column keeps the warning: %+v", got)
	}
}

// Catches: an enum value matched anywhere a quoted literal appears, so a
// COMMENT or an unrelated string equal to the value fails the migration.
func TestMigrateLintEnumUseNeedsAValuePosition(t *testing.T) {
	add := Statement{SQL: "ALTER TYPE status ADD VALUE 'archived';"}
	for _, ok := range []string{
		"COMMENT ON TYPE status IS 'archived';",
		"SELECT 'archived' AS label;",
	} {
		if got := LintSQL("v", []Statement{add, {SQL: ok}}, nil, "postgres"); len(got) != 0 {
			t.Errorf("%q: unexpected findings %+v", ok, got)
		}
	}
	for _, bad := range []string{
		"UPDATE docs SET status = 'archived';",
		"INSERT INTO docs (id, status) VALUES (1, 'archived');",
		"SELECT * FROM docs WHERE status IN ('open', 'archived');",
		"ALTER TABLE docs ALTER COLUMN status SET DEFAULT 'archived';",
	} {
		if got := LintSQL("v", []Statement{add, {SQL: bad}}, nil, "postgres"); len(got) != 1 || got[0].Rule != RuleEnumAddAndUse {
			t.Errorf("%q: want enum-add-and-use, got %+v", bad, got)
		}
	}
}

// Catches: a schema-qualified statement matched against a table of another
// schema because the lookup fell back to the bare name.
func TestMigrateLintMatchesTheSchema(t *testing.T) {
	stmt := []Statement{{SQL: "ALTER TABLE billing.docs ADD COLUMN tenant bigint NOT NULL;"}}
	if got := LintSQL("v", stmt, map[string]bool{"docs": true, "app.docs": true}, "postgres"); len(got) != 0 {
		t.Fatalf("billing.docs does not exist before this migration: %+v", got)
	}
	if got := LintSQL("v", stmt, map[string]bool{"billing.docs": true}, "postgres"); len(got) != 1 {
		t.Fatalf("billing.docs exists: %+v", got)
	}
	bare := []Statement{{SQL: "ALTER TABLE docs ADD COLUMN tenant bigint NOT NULL;"}}
	if got := LintSQL("v", bare, map[string]bool{"docs": true}, "postgres"); len(got) != 1 {
		t.Fatalf("an unqualified name resolves by name: %+v", got)
	}
}
