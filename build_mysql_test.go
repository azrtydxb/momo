// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func assignForTest(col string) Assignment { return Assignment{col: col, kind: setExcluded} }

func TestMySQLUpsertRendering(t *testing.T) {
	render := func(flavor dialect.Flavor) string {
		b := NewBuilder(dialect.MySQLFlavor(flavor))
		b.WriteString("INSERT INTO `users` (`id`, `email`) VALUES (?, ?)")
		sets := []Assignment{assignForTest("email")}
		renderMySQLUpsert(b, flavor, "users", "id", sets, false, true, func(b *Builder, a Assignment) { b.writeExcluded(a.col) })
		return b.String()
	}
	want := "INSERT INTO `users` (`id`, `email`) VALUES (?, ?) AS `momo_new` ON DUPLICATE KEY UPDATE `email` = `momo_new`.`email`"
	if got := render(dialect.FlavorMySQL); got != want {
		t.Fatalf("MySQL upsert:\n got %s\nwant %s", got, want)
	}
	if got, want := render(dialect.FlavorMariaDB), "ON DUPLICATE KEY UPDATE `email` = VALUES(`email`)"; !strings.HasSuffix(got, want) || strings.Contains(got, "momo_new") {
		t.Fatalf("MariaDB upsert = %s, want suffix %s and no row alias", got, want)
	}
	b := NewBuilder(dialect.MySQLFlavor(dialect.FlavorMySQL))
	renderMySQLUpsert(b, dialect.FlavorMySQL, "users", "id", nil, true, true, nil)
	if got, want := b.String(), " AS `momo_new` ON DUPLICATE KEY UPDATE `id` = `users`.`id`"; got != want {
		t.Fatalf("DoNothing = %s, want %s", got, want)
	}
}

func TestMySQLConflictTargetRule(t *testing.T) {
	pk := []string{"id"}
	only := &schema.Table{Name: "users", PrimaryKey: []string{"id"}}
	if err := checkMySQLConflictTarget(only, []string{"id"}, pk, nil); err != nil {
		t.Fatalf("sole primary key target rejected: %v", err)
	}
	onlyUnique := &schema.Table{Name: "users", Uniques: []schema.Unique{{Name: "users_email", Columns: []string{"email"}}}}
	if err := checkMySQLConflictTarget(onlyUnique, []string{"email"}, pk, nil); err != nil {
		t.Fatalf("sole unique target rejected: %v", err)
	}
	// Catches: a target accepted although the primary key is a second unique key
	// the statement would also fire on.
	both := &schema.Table{Name: "users", PrimaryKey: []string{"id"},
		Uniques: []schema.Unique{{Name: "users_email", Columns: []string{"email"}}}}
	for _, target := range [][]string{{"email"}, {"id"}} {
		if err := checkMySQLConflictTarget(both, target, pk, nil); !errors.Is(err, ErrUnsupported) {
			t.Errorf("PK plus unique key, target %v: err = %v, want ErrUnsupported", target, err)
		}
	}
	// A unique index counts like a unique constraint.
	withIndex := &schema.Table{Name: "users", PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "ix", Unique: true, Parts: []schema.IndexPart{{Column: "handle"}}}}}
	if err := checkMySQLConflictTarget(withIndex, []string{"id"}, pk, nil); !errors.Is(err, ErrUnsupported) {
		t.Errorf("PK plus unique index: err = %v, want ErrUnsupported", err)
	}
	if err := checkMySQLConflictTarget(nil, []string{"id"}, pk, nil); err != nil {
		t.Errorf("no schema, primary key target: %v", err)
	}
	if err := checkMySQLConflictTarget(nil, []string{"email"}, pk, nil); !errors.Is(err, ErrUnsupported) {
		t.Errorf("no schema, non-key target: err = %v, want ErrUnsupported", err)
	}
}

// Catches: the secondary-target relaxation widening past the safe case: a
// second unique key is tolerated only when every one of its columns is an
// auto-increment column the insert leaves to the database.
func TestMySQLConflictTargetSecondaryKeyRule(t *testing.T) {
	pk := []string{"id"}
	ts := &schema.Table{Name: "users", PrimaryKey: []string{"id"},
		Uniques: []schema.Unique{{Name: "users_email", Columns: []string{"email"}}}}
	if err := checkMySQLConflictTarget(ts, []string{"email"}, pk, []string{"id"}); err != nil {
		t.Errorf("auto-increment PK omitted, UNIQUE(email) target rejected: %v", err)
	}
	// The key is written, so it can match an existing row.
	if err := checkMySQLConflictTarget(ts, []string{"email"}, pk, nil); !errors.Is(err, ErrUnsupported) {
		t.Errorf("PK supplied: err = %v, want ErrUnsupported", err)
	}
	// A different column is left to the database, not the key.
	if err := checkMySQLConflictTarget(ts, []string{"email"}, pk, []string{"seq"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("other column unsupplied: err = %v, want ErrUnsupported", err)
	}
	// An ordinary second unique key stays refused, whatever else is omitted.
	two := &schema.Table{Name: "users", PrimaryKey: []string{"id"}, Uniques: []schema.Unique{
		{Name: "users_email", Columns: []string{"email"}}, {Name: "users_handle", Columns: []string{"handle"}}}}
	err := checkMySQLConflictTarget(two, []string{"email"}, pk, []string{"id"})
	if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "[handle]") {
		t.Errorf("second ordinary key: err = %v, want ErrUnsupported naming [handle]", err)
	}
	// A composite key that mixes an auto-increment column with another one is not made only of generated columns.
	mixed := &schema.Table{Name: "users", PrimaryKey: []string{"id"}, Uniques: []schema.Unique{
		{Name: "users_email", Columns: []string{"email"}}, {Name: "users_pair", Columns: []string{"id", "tenant"}}}}
	if err := checkMySQLConflictTarget(mixed, []string{"email"}, pk, []string{"id"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("composite key with an ordinary column: err = %v, want ErrUnsupported", err)
	}
	// A target that is not a unique key at all is refused with the reason.
	err = checkMySQLConflictTarget(ts, []string{"name"}, pk, []string{"id"})
	if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "not a unique key") {
		t.Errorf("non-key target: err = %v, want ErrUnsupported saying it is not a unique key", err)
	}
	// An expression key has no column and is never treated as generated.
	expr := &schema.Table{Name: "users", PrimaryKey: []string{"id"}, Uniques: []schema.Unique{{Name: "users_email", Columns: []string{"email"}}},
		Indexes: []schema.Index{{Name: "ix", Unique: true, Parts: []schema.IndexPart{{Expr: "lower(name)"}}}}}
	if err := checkMySQLConflictTarget(expr, []string{"email"}, pk, []string{"id"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("expression unique index: err = %v, want ErrUnsupported", err)
	}
}
