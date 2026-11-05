// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// InspectOptions selects what Inspect reads.
type InspectOptions struct {
	Schemas []string
	Exclude []string
}

// Dialect is the per-database migration backend.
type Dialect interface {
	Dialect() *dialect.Dialect
	Inspect(ctx context.Context, c momo.PinnedConn, opts InspectOptions) (*schema.Schema, error)
	Render(plan []Change) ([]Statement, error)
	HistoryDDL() []string
	Setup(ctx context.Context, c momo.PinnedConn, s SessionOptions) (reset func() error, err error)
	Lock(ctx context.Context, c momo.PinnedConn, key string) (unlock func() error, holder func(context.Context) (string, error), err error)
}
