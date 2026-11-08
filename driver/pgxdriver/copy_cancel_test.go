// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type errSender struct{ err error }

func (s errSender) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, s.err
}

// TestCopyCancelReportsContextError catches a cancelled COPY that surfaces
// pgx's raw network error instead of the context error. pgx's writer
// goroutine closes the socket after the cancel deadline hits, and the main
// loop then fails with "use of closed network connection" or an i/o timeout
// depending on timing, so the driver maps both to the context error.
func TestCopyCancelReportsContextError(t *testing.T) {
	closed := &net.OpError{Op: "write", Err: net.ErrClosed}
	timeout := &net.OpError{Op: "write", Err: os.ErrDeadlineExceeded}
	for name, raw := range map[string]error{"closed connection": closed, "i/o timeout": timeout} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := copyFromSource(ctx, errSender{raw}, "t", []string{"id"}, pgx.CopyFromRows(nil))
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		})
	}
}

// A server error stays a server error even when the context ended, and a
// network error with a live context is not rewritten.
func TestCopyCancelKeepsOtherErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pe := &pgconn.PgError{Code: "23505", Message: "duplicate key"}
	if _, err := copyFromSource(ctx, errSender{pe}, "t", []string{"id"}, pgx.CopyFromRows(nil)); errors.Is(err, context.Canceled) {
		t.Fatalf("server error became a cancellation: %v", err)
	}
	live := &net.OpError{Op: "write", Err: net.ErrClosed}
	if _, err := copyFromSource(t.Context(), errSender{live}, "t", []string{"id"}, pgx.CopyFromRows(nil)); errors.Is(err, context.Canceled) {
		t.Fatalf("network error with a live context became a cancellation: %v", err)
	}
}
