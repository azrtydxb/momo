// SPDX-License-Identifier: Apache-2.0

package graphql_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph/generated"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph/model"
)

// failingRoot is a root resolver whose documents query returns err, as a
// resolver does when its statement fails.
type failingRoot struct{ err *error }

func (f failingRoot) Query() generated.QueryResolver { return failingQuery{err: f.err} }

type failingQuery struct {
	generated.QueryResolver
	err *error
}

func (q failingQuery) Documents(context.Context, *int, *string, *model.DocumentFilter, *model.DocumentOrder) (*model.DocumentConnection, error) {
	return nil, *q.err
}

// postErrors posts a documents query and returns the message of its one error.
func postErrors(t *testing.T, url string) string {
	t.Helper()
	_, errs := postRaw(t, url, `query { documents { edges { cursor } } }`)
	if len(errs) != 1 {
		t.Fatalf("want one error, got %v", errs)
	}
	return errs[0]
}

// Catches: an error of the database reaching the client with its text (the
// table and the constraint name), because gqlgen wraps every resolver error in
// a gqlerror whose message is the error text and the presenter let any such
// error through; and a mapping that never runs.
func TestPresenterHidesDatabaseErrors(t *testing.T) {
	var current error
	srv := httptest.NewServer(model.NewHandler(generated.NewExecutableSchema(generated.Config{Resolvers: failingRoot{&current}})))
	t.Cleanup(srv.Close)

	leak := &momo.Error{Kind: momo.ErrUniqueViolation, Op: "insert", Table: "users", Err: errors.New(`duplicate key value violates unique constraint "users_email_key"`)}
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"unique":      {leak, "a unique constraint refused the change"},
		"foreign key": {&momo.Error{Kind: momo.ErrForeignKeyViolation, Op: "insert", Table: "comments", Err: errors.New(`violates foreign key constraint "comments_document_id_fkey"`)}, "a foreign key constraint refused the change"},
		"check":       {&momo.Error{Kind: momo.ErrCheckViolation, Op: "update", Table: "documents", Err: errors.New(`violates check constraint "documents_title_nonempty"`)}, "a constraint refused the value"},
		"busy":        {&momo.Error{Kind: momo.ErrBusy, Op: "select", Table: "documents", Err: errors.New("database is locked")}, "temporarily unavailable; retry"},
		"wrapped":     {errors.Join(errors.New("context"), leak), "a unique constraint refused the change"},
		"plain":       {errors.New(`pq: relation "secrets" does not exist`), "internal error"},
		"input":       {&model.InputError{Msg: "in takes at most 100 values"}, "in takes at most 100 values"},
	} {
		current = c.err
		msg := postErrors(t, srv.URL)
		if msg != c.want {
			t.Errorf("%s: the client sees %q, want %q", name, msg, c.want)
		}
		for _, secret := range []string{"users_email_key", "comments_document_id_fkey", "documents_title_nonempty", "secrets", "database is locked", "momo:"} {
			if strings.Contains(msg, secret) {
				t.Errorf("%s: the message leaks %q: %q", name, secret, msg)
			}
		}
	}
}
