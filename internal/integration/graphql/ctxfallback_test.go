// SPDX-License-Identifier: Apache-2.0

package graphql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph/generated"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph/model"
)

// endingRoot is a root resolver whose documents query ends the request's
// context (through *end) and then fails with an error that does not wrap the
// context error, as a driver can when it reports its own text for an aborted
// statement.
type endingRoot struct{ end *func(context.Context) }

func (e endingRoot) Query() generated.QueryResolver { return endingQuery{end: e.end} }

type endingQuery struct {
	generated.QueryResolver
	end *func(context.Context)
}

func (q endingQuery) Documents(ctx context.Context, _ *int, _ *string, _ *model.DocumentFilter, _ *model.DocumentOrder) (*model.DocumentConnection, error) {
	(*q.end)(ctx)
	return nil, errors.New("driver: statement aborted")
}

// Catches: an error that does not wrap the context error, returned after the
// request's context ended, presented as INTERNAL: the gRPC and OpenAPI servers
// look at the context as a last resort and GraphQL must agree.
func TestPresenterUsesTheContextAsAFallback(t *testing.T) {
	var end func(context.Context)
	h := model.NewHandler(generated.NewExecutableSchema(generated.Config{Resolvers: endingRoot{&end}}))

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
}
