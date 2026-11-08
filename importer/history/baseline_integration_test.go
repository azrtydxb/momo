// SPDX-License-Identifier: Apache-2.0

//go:build integration

package history

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
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
