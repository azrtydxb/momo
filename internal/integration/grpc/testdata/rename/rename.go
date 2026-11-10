// SPDX-License-Identifier: Apache-2.0

// Package rename is the example Document model with its Title field renamed to
// Name. The column stays title, so the protobuf field keeps its number.
package rename

import (
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

// Document is the example Document with Title renamed to Name.
type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64         `momo:"pk,identity"`
	TenantID   int64         `momo:"column:tenant_id,notnull,index"`
	Name       string        `momo:"column:title,size:300,notnull"`
	Body       string        `momo:"type:text"`
	Embedding  vector.Vector `momo:"dim:1536"`
	Status     string        `momo:"column:status,default:'draft'"`
	CreatedAt  time.Time     `momo:"column:created_at,read_only,default:now()"`
}
