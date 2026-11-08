// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
)

// TestMySQLLockHolder: a first apply holds the migration lock for three
// seconds; a contender with a one-second lock wait must fail with ErrLocked,
// and the error must name the connection that holds the lock.
func TestMySQLLockHolder(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	dir := t.TempDir()
	writeFile(t, dir, "20260101000000_hold.up.sql", "SELECT SLEEP(3);\n")
	holder := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
	contender := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))

	done := make(chan error, 1)
	go func() {
		_, err := holder.Apply(ctx, migrate.ApplyOptions{})
		done <- err
	}()
	time.Sleep(500 * time.Millisecond) // let the holder take the lock first

	_, err := contender.Apply(ctx, migrate.ApplyOptions{LockWait: time.Second})
	if !errors.Is(err, migrate.ErrLocked) {
		t.Fatalf("contender apply: err = %v, want migrate.ErrLocked", err)
	}
	if !strings.Contains(err.Error(), "held by connection") {
		t.Fatalf("the error should name the holding connection: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("holder apply: %v", err)
	}
	// The lock is free again once the holder is done.
	if _, err := contender.Apply(ctx, migrate.ApplyOptions{LockWait: time.Second}); err != nil {
		t.Fatalf("apply after the holder finished: %v", err)
	}
}
