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
		renderMySQLUpsert(b, flavor, "id", sets, false, true, func(b *Builder, a Assignment) { b.writeExcluded(a.col) })
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
	renderMySQLUpsert(b, dialect.FlavorMySQL, "id", nil, true, true, nil)
	if got, want := b.String(), " AS `momo_new` ON DUPLICATE KEY UPDATE `id` = `id`"; got != want {
		t.Fatalf("DoNothing = %s, want %s", got, want)
	}
}

func TestMySQLConflictTargetRule(t *testing.T) {
	pk := []string{"id"}
	only := &schema.Table{Name: "users", PrimaryKey: []string{"id"}}
	if err := checkMySQLConflictTarget(only, []string{"id"}, pk); err != nil {
		t.Fatalf("sole primary key target rejected: %v", err)
	}
	onlyUnique := &schema.Table{Name: "users", Uniques: []schema.Unique{{Name: "users_email", Columns: []string{"email"}}}}
	if err := checkMySQLConflictTarget(onlyUnique, []string{"email"}, pk); err != nil {
		t.Fatalf("sole unique target rejected: %v", err)
	}
	// Catches: a target accepted although the primary key is a second unique key
	// the statement would also fire on.
	both := &schema.Table{Name: "users", PrimaryKey: []string{"id"},
		Uniques: []schema.Unique{{Name: "users_email", Columns: []string{"email"}}}}
	for _, target := range [][]string{{"email"}, {"id"}} {
		if err := checkMySQLConflictTarget(both, target, pk); !errors.Is(err, ErrUnsupported) {
			t.Errorf("PK plus unique key, target %v: err = %v, want ErrUnsupported", target, err)
		}
	}
	// A unique index counts like a unique constraint.
	withIndex := &schema.Table{Name: "users", PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "ix", Unique: true, Parts: []schema.IndexPart{{Column: "handle"}}}}}
	if err := checkMySQLConflictTarget(withIndex, []string{"id"}, pk); !errors.Is(err, ErrUnsupported) {
		t.Errorf("PK plus unique index: err = %v, want ErrUnsupported", err)
	}
	if err := checkMySQLConflictTarget(nil, []string{"id"}, pk); err != nil {
		t.Errorf("no schema, primary key target: %v", err)
	}
	if err := checkMySQLConflictTarget(nil, []string{"email"}, pk); !errors.Is(err, ErrUnsupported) {
		t.Errorf("no schema, non-key target: err = %v, want ErrUnsupported", err)
	}
}
