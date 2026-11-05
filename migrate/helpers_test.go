// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
)

func pinned(t *testing.T, db *momo.DB) momo.PinnedConn {
	t.Helper()
	pc, err := db.Driver().(momo.Pinner).Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Release() })
	return pc
}

func mustExec(t *testing.T, c momo.PinnedConn, sql string) {
	t.Helper()
	if _, err := c.Exec(context.Background(), sql, nil); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func historyRow(t *testing.T, db *momo.DB, version string) migrate.HistoryRow {
	t.Helper()
	c := pinned(t, db)
	rows, err := migrate.ReadHistory(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Version == version {
			return r
		}
	}
	t.Fatalf("no history row for %s in %+v", version, rows)
	return migrate.HistoryRow{}
}

func tableExists(t *testing.T, db *momo.DB, name string) bool {
	t.Helper()
	rows, err := pinned(t, db).Query(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, []any{name})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ok bool
	if !rows.Next() {
		t.Fatal("no row")
	}
	if err := rows.Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}
