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
	ts := &schema.Table{Name: "users", PrimaryKey: []string{"id"},
		Uniques: []schema.Unique{{Name: "users_email", Columns: []string{"email"}}}}
	pk := []string{"id"}
	if err := checkMySQLConflictTarget(ts, []string{"email"}, pk); err != nil {
		t.Fatalf("sole unique target rejected: %v", err)
	}
	if err := checkMySQLConflictTarget(ts, []string{"id"}, pk); err != nil {
		t.Fatalf("primary key target rejected: %v", err)
	}
	ts.Uniques = append(ts.Uniques, schema.Unique{Name: "users_handle", Columns: []string{"handle"}})
	if err := checkMySQLConflictTarget(ts, []string{"email"}, pk); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("second unique key: err = %v, want ErrUnsupported", err)
	}
	if err := checkMySQLConflictTarget(nil, []string{"email"}, pk); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("no schema, non-key target: err = %v, want ErrUnsupported", err)
	}
}
