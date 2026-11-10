// SPDX-License-Identifier: Apache-2.0

package openapi_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/openapi/example/handlers"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: an error answered 500 with nothing for the application to log
// (the hook not called, called twice, or called after the message), a hook that
// receives the fixed message instead of the original error, and a database
// error leaking to the client.
func TestOnErrorHookSeesTheOriginalError(t *testing.T) {
	db := testopen.Postgres(t)
	fixture.Apply(t, db)
	mux := serve(t, db)
	handlers.Seen.Reset()

	// A missing row: one call, with the sentinel and the operation.
	rec := get(context.Background(), mux, "/v1/documents/999999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing row: status %d, want 404", rec.Code)
	}
	seen := handlers.Seen.Entries()
	if len(seen) != 1 || seen[0].Op != "getDocument" || !errors.Is(seen[0].Err, momo.ErrNotFound) {
		t.Fatalf("hook saw %+v, want one getDocument with ErrNotFound", seen)
	}

	// A failing statement: the table is gone, so the error is the driver's.
	handlers.Seen.Reset()
	if _, err := momo.Exec(context.Background(), db, `ALTER TABLE tags RENAME TO tags_moved`); err != nil {
		t.Fatal(err)
	}
	rec = get(context.Background(), mux, "/v1/tags/1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("failing statement: status %d, want 500: %s", rec.Code, rec.Body)
	}
	if msg := errorOf(t, rec); msg != "internal error" {
		t.Fatalf("the client sees %q, want the fixed message", msg)
	}
	if strings.Contains(rec.Body.String(), "tags") {
		t.Fatalf("the response names the table: %s", rec.Body)
	}
	seen = handlers.Seen.Entries()
	if len(seen) != 1 || seen[0].Op != "getTag" {
		t.Fatalf("hook saw %+v, want one getTag", seen)
	}
	if !strings.Contains(seen[0].Err.Error(), "tags") {
		t.Fatalf("the hook did not get the original error: %v", seen[0].Err)
	}
}
