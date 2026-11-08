// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
)

// TestMySQLOnlineDDL: the -- momo:ddl directive appends ALGORITHM and LOCK to
// the ALTER TABLE after it. An instant add column succeeds on a table with
// 100000 rows; a change the server cannot do instantly is rejected, and the
// history records no applied statement.
func TestMySQLOnlineDDL(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	mustExec(t, db, "CREATE TABLE big (id bigint PRIMARY KEY)")
	mustExec(t, db, "INSERT INTO big (id) SELECT seq FROM (SELECT @r := @r + 1 AS seq FROM information_schema.columns a, information_schema.columns b, (SELECT @r := 0) init LIMIT 100000) s")

	dir := t.TempDir()
	writeFile(t, dir, "20260101000000_add_note.up.sql",
		"-- momo:ddl ALGORITHM=INSTANT, LOCK=NONE\nALTER TABLE big ADD COLUMN note varchar(40) NULL;\n")
	writeFile(t, dir, "20260101000001_bad_alter.up.sql",
		"-- momo:ddl ALGORITHM=INSTANT\nALTER TABLE big MODIFY COLUMN id bigint unsigned;\n")

	eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{To: "20260101000000"}); err != nil {
		t.Fatalf("instant add column: %v", err)
	}
	if _, err := momo.Exec(ctx, db, "SELECT note FROM big LIMIT 1"); err != nil {
		t.Fatalf("column note missing after the directive migration: %v", err)
	}

	_, err := eng.Apply(ctx, migrate.ApplyOptions{})
	if err == nil {
		t.Fatal("an unsupported instant change was applied")
	}
	// The driver withholds server text from Error(); the server message is on
	// the cause.
	var me *gomysql.MySQLError
	if !errors.As(err, &me) || !strings.Contains(me.Message, "ALGORITHM=INSTANT is not supported") {
		t.Fatalf("unsupported instant change: err = %v, cause = %v", err, me)
	}
	if got := appliedStatements(t, db, "20260101000001"); got != 0 {
		t.Fatalf("statements_applied for the rejected migration = %d, want 0", got)
	}
}
