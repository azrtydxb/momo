// SPDX-License-Identifier: Apache-2.0

// Package models is the schema of the upgrade-database example, at its latest
// version. Each version of the schema is one migration in ../migrations.
package models

import "github.com/azrtydxb/momo"

// Customer is one account.
type Customer struct {
	momo.Model `momo:"table:customers"`
	ID         int64  `momo:"pk,identity"`
	Email      string `momo:"size:200,notnull,unique"`
	Name       string `momo:"column:full_name,size:200,notnull"`
	// Plan was added in version 2. NOT NULL with a default is safe on a table
	// that already has rows: existing rows take the default.
	Plan string `momo:"size:20,notnull,default:'free',index"`
}
