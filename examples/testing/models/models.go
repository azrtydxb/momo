// SPDX-License-Identifier: Apache-2.0

// Package models is the schema of the testing example: bank accounts
// whose balance the database refuses to take below zero.
package models

import "github.com/azrtydxb/momo"

// Account holds money. The check constraint is what makes a failed debit a
// reliable reason to roll back: the database enforces the rule, not the
// caller.
type Account struct {
	momo.Model `momo:"table:accounts"`
	ID         int64  `momo:"pk,identity"`
	Owner      string `momo:"size:100,notnull,unique"`
	Balance    int64  `momo:"notnull,check:balance >= 0"`
}
