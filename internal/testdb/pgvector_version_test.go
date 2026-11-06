//go:build integration

// SPDX-License-Identifier: Apache-2.0

package testdb

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestPgvectorVersionPinned asserts that the database under test runs the
// pinned pgvector extension. CI runs it against both pinned images.
func TestPgvectorVersionPinned(t *testing.T) {
	conn, err := pgx.Connect(t.Context(), PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	var v string
	if err := conn.QueryRow(t.Context(), `SELECT extversion FROM pg_extension WHERE extname = 'vector'`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != pinnedPgvector {
		t.Fatalf("pgvector extversion = %s, want %s (see docs/vectors.md)", v, pinnedPgvector)
	}
}
