// SPDX-License-Identifier: Apache-2.0

package googleuuid

import (
	"github.com/google/uuid"

	"github.com/azrtydxb/momo"
)

// Member uses github.com/google/uuid types with no type tag.
type Member struct {
	momo.Model `momo:"table:members"`
	ID         uuid.UUID   `momo:"pk"`
	OwnerID    *uuid.UUID  `momo:"column:owner_id,nullable"`
	TeamIDs    []uuid.UUID `momo:"column:team_ids,notnull"`
}
