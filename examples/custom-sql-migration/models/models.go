// SPDX-License-Identifier: Apache-2.0

// Package models is the schema of the custom-sql-migration example.
package models

import "github.com/azrtydxb/momo"

// Account is a login. EmailNorm is the lower-cased email, used for lookups
// that ignore case. The unique index on it is added last, after a migration
// has filled the column in.
type Account struct {
	momo.Model `momo:"table:accounts"`
	ID         int64  `momo:"pk,identity"`
	Email      string `momo:"size:200,notnull"`
	EmailNorm  string `momo:"column:email_norm,size:200,notnull,default:'',unique"`
}
