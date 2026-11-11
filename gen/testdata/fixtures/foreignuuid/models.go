// SPDX-License-Identifier: Apache-2.0

package foreignuuid

import (
	other "github.com/azrtydxb/momo/gen/testdata/fixtures/foreignuuid/uuid"

	"github.com/azrtydxb/momo"
)

// Thing holds UUID types of a package that is not github.com/google/uuid.
type Thing struct {
	momo.Model `momo:"table:things"`
	ID         int64        `momo:"pk,identity"`
	V          other.UUID   `momo:"column:v"`
	Vs         []other.UUID `momo:"column:vs,notnull"`
}
