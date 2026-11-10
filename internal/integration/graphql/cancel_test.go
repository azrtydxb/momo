// SPDX-License-Identifier: Apache-2.0

package graphql_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph/generated"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph/model"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

type gqlError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions"`
}

// serveQuery runs the documents query through h with the context ctx and
// returns the errors of the response.
func serveQuery(t *testing.T, ctx context.Context, h http.Handler) []gqlError {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"query":"query { documents { edges { cursor } } }"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out struct {
		Errors []gqlError `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return out.Errors
}

// Catches: a resolver error that is a context error presented as INTERNAL,
// which a client reads as a server fault, or with the text of the cause.
func TestPresenterMapsContextErrors(t *testing.T) {
	var current error
	h := model.NewHandler(generated.NewExecutableSchema(generated.Config{Resolvers: failingRoot{&current}}))
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"canceled":         {context.Canceled, "request canceled"},
		"deadline":         {context.DeadlineExceeded, "request timed out"},
		"wrapped canceled": {&momo.Error{Kind: momo.ErrCanceled, Op: "select", Table: "documents", Err: context.Canceled}, "request canceled"},
		"joined deadline":  {errors.Join(errors.New("select documents"), context.DeadlineExceeded), "request timed out"},
	} {
		current = c.err
		errs := serveQuery(t, context.Background(), h)
		if len(errs) != 1 || errs[0].Message != c.want || errs[0].Extensions["code"] != "CANCELED" {
			t.Errorf("%s: got %+v, want one CANCELED %q", name, errs, c.want)
		}
	}
}

// Catches: a request cancelled in the middle of a resolver, or whose deadline
// passed there, answered INTERNAL.
func TestCancelledRequestIsCanceled(t *testing.T) {
	var end func(ctx context.Context)
	db := testopen.Postgres(t, momo.WithInterceptor(func(ctx context.Context, op *momo.Operation) error {
		if op.Kind == "select" && end != nil {
			end(ctx)
		}
		return nil
	}))
	fixture.Apply(t, db)
	h := model.NewHandler(generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{DB: db}}))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	end = func(context.Context) { cancel() }
	errs := serveQuery(t, ctx, h)
	if len(errs) != 1 || errs[0].Message != "request canceled" || errs[0].Extensions["code"] != "CANCELED" {
		t.Fatalf("cancelled: got %+v, want one CANCELED \"request canceled\"", errs)
	}

	ctx, cancelTimeout := context.WithTimeout(context.Background(), 20*time.Millisecond)
	t.Cleanup(cancelTimeout)
	end = func(ctx context.Context) { <-ctx.Done() }
	errs = serveQuery(t, ctx, h)
	if len(errs) != 1 || errs[0].Message != "request timed out" || errs[0].Extensions["code"] != "CANCELED" {
		t.Fatalf("deadline: got %+v, want one CANCELED \"request timed out\"", errs)
	}
	end = nil
}
