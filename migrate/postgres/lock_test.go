// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"
	"strings"

	"github.com/azrtydxb/momo"
)

// failingConn fails statements containing failOn, to force the unlock and reset error paths.
type failingConn struct {
	momo.PinnedConn
	failOn    string
	discarded bool
}

func (f *failingConn) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	if strings.Contains(sql, f.failOn) {
		return 0, errors.New("forced failure")
	}
	return f.PinnedConn.Exec(ctx, sql, args)
}

func (f *failingConn) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	if strings.Contains(sql, f.failOn) {
		return nil, errors.New("forced failure")
	}
	return f.PinnedConn.Query(ctx, sql, args)
}

func (f *failingConn) Discard() error {
	f.discarded = true
	return f.PinnedConn.(momo.Discarder).Discard()
}
