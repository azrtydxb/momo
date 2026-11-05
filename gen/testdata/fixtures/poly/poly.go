// SPDX-License-Identifier: Apache-2.0

package poly

import "github.com/azrtydxb/momo"

type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64   `momo:"pk,identity"`
	Images     []Image `momo:"rel:has_many,fk:OwnerID,polytype:owner_type,polyvalue:document"`
}

type Image struct {
	momo.Model `momo:"table:images"`
	ID         int64  `momo:"pk,identity"`
	OwnerID    int64  `momo:"column:owner_id"`
	OwnerType  string `momo:"column:owner_type"`
}
