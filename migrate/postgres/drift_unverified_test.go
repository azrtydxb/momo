// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"testing/fstest"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
)

// Drift does not compare view queries, trigger conditions or policy
// expressions, so the report names those objects instead of reassuring the
// caller: a body changed outside the migrations leaves the report Clean, and
// Unverified lists what was not compared.
func TestDriftListsObjectsWhoseBodiesWereNotCompared(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	createDeclaredLive(t, db.Driver())
	var snap bytes.Buffer
	if err := migrate.WriteSnapshot(&snap, migrate.SnapshotFile{Version: "20260101000000", Schema: declaredObjects()}); err != nil {
		t.Fatal(err)
	}
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_init.up.sql":        {Data: []byte("SELECT 1;")},
		"20260101000000_init.snapshot.json": {Data: snap.Bytes()},
	})
	e := engineOn(t, db, fsys)
	if err := e.Baseline(ctx, "20260101000000", false); err != nil {
		t.Fatal(err)
	}
	// Change the policy and a view body out of band.
	mustExec(t, db.Driver(), `DROP POLICY docs_pos ON docs`)
	mustExec(t, db.Driver(), `CREATE POLICY docs_pos ON docs FOR SELECT USING (true)`)
	mustExec(t, db.Driver(), `CREATE OR REPLACE VIEW v_docs AS SELECT id, title FROM docs`)
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Fatalf("bodies are not compared, so the report stays clean: %+v", rep)
	}
	var got []string
	for _, r := range rep.Unverified {
		got = append(got, r.Kind+":"+r.Name)
	}
	want := []string{"policy:docs.docs_pos", "trigger:docs.docs_touch", "view:m_count", "view:v_docs", "view:v_titles"}
	if !slices.Equal(got, want) {
		t.Fatalf("Unverified = %v, want %v", got, want)
	}
}
