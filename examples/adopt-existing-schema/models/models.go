// SPDX-License-Identifier: Apache-2.0

// Package models describes a schema that already exists in a database. The
// tables were created by hand-written SQL (see ../legacy.sql); these structs
// are written to match them column for column.
package models

import (
	"time"

	"github.com/azrtydxb/momo"
)

// User is a registered customer.
type User struct {
	momo.Model `momo:"table:users"`
	ID         int64     `momo:"pk,identity"`
	Email      string    `momo:"size:200,notnull,unique"`
	CreatedAt  time.Time `momo:"column:created_at,read_only,default:now()"`

	Orders []Order `momo:"has_many,fk:UserID"`
}

// Order is one purchase.
type Order struct {
	momo.Model `momo:"table:orders"`
	ID         int64     `momo:"pk,identity"`
	UserID     int64     `momo:"column:user_id,notnull,index"`
	TotalCents int64     `momo:"column:total_cents,notnull"`
	PlacedAt   time.Time `momo:"column:placed_at,read_only,default:now()"`
	// Status is the first change made through Momo, after the schema was
	// adopted. It is not in legacy.sql.
	Status string `momo:"size:20,notnull,default:'new'"`

	User *User `momo:"belongs_to,fk:UserID"`
}
