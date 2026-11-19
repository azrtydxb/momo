// SPDX-License-Identifier: Apache-2.0

// Package embedfrom exercises the embed_from tag: a vector field whose
// embedding is kept in sync with its source columns. A source is named by its
// column or its Go name.
package embedfrom

import (
	"database/sql"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

type Doc struct {
	momo.Model `momo:"table:docs"`
	ID         int64          `momo:"pk,identity"`
	Title      string         `momo:"notnull"`
	Body       *string        `momo:"size:2000"`
	Summary    sql.NullString `momo:"column:summary"`
	Embedding  *vector.Vector `momo:"dim:3,embed_from:title|Body|summary"`
}
