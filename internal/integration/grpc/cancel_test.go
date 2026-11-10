// SPDX-License-Identifier: Apache-2.0

package grpc_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/grpc/example"
	"github.com/azrtydxb/momo/internal/integration/grpc/example/pb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: a call the client cancelled, or whose deadline passed, answered
// Internal.
func TestCancelledCallIsNotInternal(t *testing.T) {
	var end func(ctx context.Context)
	db := testopen.Postgres(t, momo.WithInterceptor(func(ctx context.Context, op *momo.Operation) error {
		if op.Kind == "select" && end != nil {
			end(ctx)
		}
		return nil
	}))
	fixture.Apply(t, db)
	docs := example.NewDocumentServer(db)

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		end = func(context.Context) { cancel() }
		_, err := docs.Get(ctx, &pb.DocumentGetRequest{Id: 1})
		if status.Code(err) != codes.Canceled || status.Convert(err).Message() != "request canceled" {
			t.Fatalf("got %v, want Canceled with the fixed message", err)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		t.Cleanup(cancel)
		end = func(ctx context.Context) { <-ctx.Done() }
		_, err := docs.List(ctx, &pb.DocumentListRequest{})
		if status.Code(err) != codes.DeadlineExceeded || status.Convert(err).Message() != "deadline exceeded" {
			t.Fatalf("got %v, want DeadlineExceeded with the fixed message", err)
		}
	})
	end = nil
}
