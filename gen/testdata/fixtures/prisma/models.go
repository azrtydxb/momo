// SPDX-License-Identifier: Apache-2.0

package prisma

import (
	"time"

	"github.com/azrtydxb/momo"
)

// Account mirrors a Prisma model with a string id, a unique email and
// DateTime columns.
type Account struct {
	momo.Model
	ID        string    `momo:"pk"`
	Email     string    `momo:"notnull,unique"`
	CreatedAt time.Time `momo:"notnull"`
	UpdatedAt time.Time `momo:"notnull"`
}

// KbChunk mirrors a Prisma model with a foreign key and a nullable DateTime.
type KbChunk struct {
	momo.Model
	ID        string    `momo:"pk"`
	AccountID string    `momo:"notnull,index"`
	Body      string    `momo:"notnull"`
	CreatedAt time.Time `momo:"notnull"`
	DeletedAt *time.Time

	Account *Account `momo:"belongs_to,fk:AccountID,on_delete:restrict,on_update:cascade"`
}
