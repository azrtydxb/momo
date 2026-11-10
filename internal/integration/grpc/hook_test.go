// SPDX-License-Identifier: Apache-2.0

package grpc_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/grpc/example"
	"github.com/azrtydxb/momo/internal/integration/grpc/example/pb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: an error answered Internal with nothing for the application to log
// (the hook not called, called twice, or given the fixed status instead of the
// original error), and a database error leaking to the client.
func TestOnErrorHookSeesTheOriginalError(t *testing.T) {
	db := testopen.Postgres(t)
	fixture.Apply(t, db)
	ctx := context.Background()
	docs := example.NewDocumentServer(db)
	tags := example.NewTagServer(db)
	example.Seen.Reset()

	_, err := docs.Get(ctx, &pb.DocumentGetRequest{Id: 999999})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("missing row: %v, want NotFound", err)
	}
	seen := example.Seen.Entries()
	if len(seen) != 1 || seen[0].Op != "DocumentService.Get" || !errors.Is(seen[0].Err, momo.ErrNotFound) {
		t.Fatalf("hook saw %+v, want one DocumentService.Get with ErrNotFound", seen)
	}

	example.Seen.Reset()
	if _, err := momo.Exec(ctx, db, `ALTER TABLE tags RENAME TO tags_moved`); err != nil {
		t.Fatal(err)
	}
	_, err = tags.Get(ctx, &pb.TagGetRequest{Id: 1})
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Fatalf("failing statement: %v, want Internal with the fixed message", err)
	}
	if strings.Contains(err.Error(), "tags") {
		t.Fatalf("the status names the table: %v", err)
	}
	seen = example.Seen.Entries()
	if len(seen) != 1 || seen[0].Op != "TagService.Get" || !strings.Contains(seen[0].Err.Error(), "tags") {
		t.Fatalf("hook saw %+v, want one TagService.Get with the original error", seen)
	}
}
