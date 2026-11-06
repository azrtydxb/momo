// SPDX-License-Identifier: Apache-2.0

package namedtype

import (
	"database/sql"
	"time"

	"github.com/azrtydxb/momo"
)

type ID int64

type Label string

type Item struct {
	momo.Model
	ID      ID
	Label   Label
	Note    *Label
	Opt     sql.NullString
	Created time.Time
}
