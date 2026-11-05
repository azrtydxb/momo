// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"strings"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
)

func TestHistoryFormatVersion(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	if err := migrate.EnsureHistory(ctx, c, migratetest.Postgres()); err != nil {
		t.Fatal(err)
	}
	mustExec(t, c, `UPDATE momo_migrations SET checksum='99' WHERE version='__momo_meta__'`)
	err := migrate.EnsureHistory(ctx, c, migratetest.Postgres())
	if err == nil || !strings.Contains(err.Error(), "upgrade momo") {
		t.Fatalf("newer history format must be refused with an actionable error, got %v", err)
	}
}
