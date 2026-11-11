// SPDX-License-Identifier: Apache-2.0

package googleuuidalias

import (
	gu "github.com/google/uuid"

	"github.com/azrtydxb/momo"
)

// ID is an alias of the google type; it is the same type.
type ID = gu.UUID

// Item reaches github.com/google/uuid through an import alias and a type alias.
type Item struct {
	momo.Model `momo:"table:items"`
	A          gu.UUID   `momo:"pk,column:a"`
	B          ID        `momo:"column:b"`
	C          []ID      `momo:"column:c,notnull"`
	D          []gu.UUID `momo:"column:d,notnull"`
}
