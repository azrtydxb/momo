// SPDX-License-Identifier: Apache-2.0

//go:build integration

package history

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBaselineSnapshotsEachMigration(t *testing.T) {
	scratch := testopen.Postgres(t)
	conv, _, err := Convert(Source{Tool: "goose", Migrations: []Raw{
		{Version: "1", Name: "a", Up: []byte("CREATE TABLE a (x int);"), Origin: "1_a.sql"},
		{Version: "2", Name: "b", Up: []byte("CREATE TABLE b (y int);\nCREATE INDEX b_y ON b (y);"), Origin: "2_b.sql"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := Baseline(context.Background(), scratch, mpg.New(), conv)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("want one snapshot per migration, got %d", len(snaps))
	}
	if snaps[0].ParentVersion != "" || snaps[1].ParentVersion != conv[0].Version {
		t.Fatalf("parent chain wrong: %q, %q", snaps[0].ParentVersion, snaps[1].ParentVersion)
	}
	if len(snaps[0].Schema.Tables) != 1 || len(snaps[1].Schema.Tables) != 2 {
		t.Fatalf("snapshots must show the schema after each migration: %d, %d tables",
			len(snaps[0].Schema.Tables), len(snaps[1].Schema.Tables))
	}
}

func TestBaselineRefusesNonEmptyScratch(t *testing.T) {
	scratch := testopen.Postgres(t)
	if _, err := momo.Exec(context.Background(), scratch, "CREATE TABLE leftover (x int)"); err != nil {
		t.Fatal(err)
	}
	if _, err := Baseline(context.Background(), scratch, mpg.New(), nil); err == nil {
		t.Fatal("Baseline must refuse a scratch database that is not empty")
	}
}

// A pool of one connection must not deadlock Baseline: it may not hold a
// connection while Inspect asks for another. The deadline turns a regression
// into a failure instead of a hang.
func TestBaselineWorksOnASingleConnectionPool(t *testing.T) {
	u, err := url.Parse(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("pool_max_conns", "1")
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	scratch := momo.New(pgxdriver.New(pool))
	conv, _, err := Convert(Source{Tool: "goose", Migrations: []Raw{
		{Version: "1", Name: "a", Up: []byte("CREATE TABLE a (x int);"), Origin: "1_a.sql"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := Baseline(ctx, scratch, mpg.New(), conv)
	if err != nil {
		t.Fatalf("Baseline on a one-connection pool: %v", err)
	}
	if len(snaps) != 1 || len(snaps[0].Schema.Tables) != 1 {
		t.Fatalf("unexpected snapshots: %+v", snaps)
	}
}
