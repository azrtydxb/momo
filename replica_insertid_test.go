// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// Catches: the replica router hiding InsertIDExecer, so a MySQL insert through
// a Replica handle reported the generated key as 0.
func TestReplicaHandleReportsInsertID(t *testing.T) {
	db := momo.New(idDriver{fakedriver.New(dialect.MySQL)}, momo.WithReplicas(fakedriver.New(dialect.MySQL)))
	rep, err := db.Replica(0)
	if err != nil {
		t.Fatal(err)
	}
	doc := suite.Document{TenantID: 1, Title: "t"}
	res, err := momo.Insert[suite.Document](rep).Model(&doc).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID != 42 || res.InsertID != 42 {
		t.Fatalf("generated key lost: model %d, result %d", doc.ID, res.InsertID)
	}
}

// A primary without InsertIDExecer still inserts through a Replica handle.
func TestReplicaHandleInsertWithoutInsertIDExecer(t *testing.T) {
	drv := fakedriver.New(dialect.MySQL)
	db := momo.New(drv, momo.WithReplicas(fakedriver.New(dialect.MySQL)))
	rep, err := db.Replica(0)
	if err != nil {
		t.Fatal(err)
	}
	doc := suite.Document{TenantID: 1, Title: "t"}
	if _, err := momo.Insert[suite.Document](rep).Model(&doc).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(drv.Calls()) != 1 {
		t.Fatalf("primary calls = %v, want the one INSERT", drv.Calls())
	}
}

// Pins that a pinned OnConn handle never yields a Replica handle (OnConn drops
// the replicas), so the pinned session cannot be lost silently.
func TestReplicaOnPinnedHandleIsUnsupported(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithReplicas(fakedriver.New(dialect.Postgres)))
	pinned := db.OnConn(copyPinned{})
	if _, err := pinned.Replica(0); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Replica on a pinned handle = %v, want ErrUnsupported", err)
	}
}
