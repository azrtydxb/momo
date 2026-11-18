// SPDX-License-Identifier: Apache-2.0

package goose

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/importer/history/internal/histtest"
)

func TestReadContextPassesTheContext(t *testing.T) {
	histtest.CheckReadContext(t, func(ctx context.Context, c momo.PinnedConn) error {
		_, err := ReadContext(ctx, fstest.MapFS{}, c, "goose_db_version")
		return err
	})
}
