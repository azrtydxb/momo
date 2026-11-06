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

	Tenant   *Tenant   `momo:"belongs_to,fk:TenantID"`
	Profile  *Profile  `momo:"has_one,fk:DocumentID"`
	Comments []Comment `momo:"has_many,fk:OwnerID,polytype:owner_type,polyvalue:document"`
	Cover    *Comment  `momo:"has_one,fk:OwnerID,polytype:owner_type,polyvalue:cover"`
	Tags     []Tag     `momo:"many2many,join:document_tags"`
}

type Profile struct {
	momo.Model `momo:"table:profiles"`
	ID         int64 `momo:"pk,identity"`
	DocumentID int64 `momo:"column:document_id,notnull"`
}

type Comment struct {
	momo.Model `momo:"table:comments"`
	ID         int64  `momo:"pk,identity"`
	OwnerID    int64  `momo:"column:owner_id,notnull"`
	OwnerType  string `momo:"column:owner_type,notnull"`
}

type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"notnull"`
}
