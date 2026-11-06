// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// stubRows fails only once it has been closed, like pgx reporting a command
// completion error after the last row.
type stubRows struct {
	pgx.Rows
	closed bool
	err    error
}

func (s *stubRows) Close() { s.closed = true }
func (s *stubRows) Err() error {
	if s.closed {
		return s.err
	}
	return nil
}

func TestPgxRowsCloseReportsErr(t *testing.T) {
	boom := errors.New("boom")
	s := &stubRows{err: boom}
	if err := (pgxRows{s}).Close(); !errors.Is(err, boom) || !s.closed {
		t.Fatalf("Close = %v, closed = %v; want the rows error", err, s.closed)
	}
	if err := (pgxRows{&stubRows{}}).Close(); err != nil {
		t.Fatalf("clean Close = %v", err)
	}
}
