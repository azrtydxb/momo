// SPDX-License-Identifier: Apache-2.0

package models

import (
	"time"

	"github.com/azrtydxb/momo"
)

type Tenant struct {
	momo.Model `momo:"table:tenants"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"size:200,notnull"`
}

type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64     `momo:"pk,identity"`
	TenantID   int64     `momo:"column:tenant_id,notnull,index"`
	Title      string    `momo:"size:300,notnull"`
	Note       *string   `momo:"column:note"`
	CreatedAt  time.Time `momo:"column:created_at,read_only"`
	Skipped    string    `momo:"-"`
}
