// SPDX-License-Identifier: Apache-2.0

// Package bunfixture is a small project written with Bun. The importer reads
// its source, and its test compares the schema Bun creates with the schema
// Momo derives from the imported models.
package bunfixture

import (
	"time"

	"github.com/uptrace/bun"
)

// Org is a tenant.
type Org struct {
	bun.BaseModel `bun:"table:orgs,alias:o"`

	ID   int64  `bun:"id,pk,autoincrement"`
	Name string `bun:"name,notnull,unique"`
}

// Account belongs to an Org.
type Account struct {
	bun.BaseModel `bun:"table:accounts"`

	ID        int64     `bun:"id,pk,autoincrement"`
	OrgID     int64     `bun:"org_id,notnull"`
	Org       *Org      `bun:"rel:belongs-to,join:org_id=id"`
	Email     string    `bun:"email,notnull,unique"`
	Balance   int64     `bun:"balance,nullzero,default:0"`
	CreatedAt time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp"`
}

// Audit is embedded into Profile under a prefix.
type Audit struct {
	CreatedBy string `bun:"created_by"`
}

// Profile covers the column types, the key actions, a soft-delete column and a
// composite unique constraint.
type Profile struct {
	bun.BaseModel `bun:"table:profiles"`

	ID        int64     `bun:"id,pk,autoincrement"`
	AccountID int64     `bun:"account_id,notnull"`
	Account   *Account  `bun:"rel:belongs-to,join:account_id=id,on_delete:cascade"`
	Bio       *string   `bun:"bio"`
	Active    bool      `bun:"active,notnull,default:true"`
	Rank      int32     `bun:"rank,notnull"`
	Avatar    []byte    `bun:"avatar"`
	Slug      string    `bun:"slug,notnull,type:varchar(80)"`
	Region    string    `bun:"region,notnull,unique:uq_profile_region_slug"`
	Code      string    `bun:"code,notnull,unique:uq_profile_region_slug"`
	DeletedAt time.Time `bun:"deleted_at,soft_delete,nullzero"`
	Audit     Audit     `bun:"embed:audit_"`
}
