// SPDX-License-Identifier: Apache-2.0

package openapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/openapi/example/handlers"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func serve(t *testing.T, db *momo.DB) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	(&handlers.DocumentHandler{DB: db}).Routes(mux)
	(&handlers.TagHandler{DB: db}).Routes(mux)
	return mux
}

func get(ctx context.Context, mux *http.ServeMux, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return e.Error
}

// Catches: a request the client cancelled, or whose deadline passed, answered
// as a server error.
func TestCancelledRequestIsNot500(t *testing.T) {
	// The interceptor ends the context of the request as its statement starts,
	// so the request is cancelled in the middle of the handler.
	var end func(ctx context.Context)
	db := testopen.Postgres(t, momo.WithInterceptor(func(ctx context.Context, op *momo.Operation) error {
		if op.Kind == "select" && end != nil {
			end(ctx)
		}
		return nil
	}))
	fixture.Apply(t, db)
	mux := serve(t, db)

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		end = func(context.Context) { cancel() }
		rec := get(ctx, mux, "/v1/documents/1")
		if rec.Code != 499 {
			t.Fatalf("status %d, want 499: %s", rec.Code, rec.Body)
		}
		if msg := errorOf(t, rec); msg != "request canceled" {
			t.Fatalf("message %q", msg)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		t.Cleanup(cancel)
		end = func(ctx context.Context) { <-ctx.Done() }
		rec := get(ctx, mux, "/v1/documents")
		if rec.Code != http.StatusGatewayTimeout {
			t.Fatalf("status %d, want 504: %s", rec.Code, rec.Body)
		}
		if msg := errorOf(t, rec); msg != "request timed out" {
			t.Fatalf("message %q", msg)
		}
	})
	end = nil
}
