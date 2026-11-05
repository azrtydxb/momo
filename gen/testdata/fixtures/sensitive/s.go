// SPDX-License-Identifier: Apache-2.0

package sensitive

import "github.com/azrtydxb/momo"

type Account struct {
	momo.Model `momo:"table:accounts"`
	ID         int64  `momo:"pk,identity"`
	Email      string `momo:"notnull"`
	Password   string `momo:"sensitive"`
}
