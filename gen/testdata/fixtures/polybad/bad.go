// SPDX-License-Identifier: Apache-2.0

package polybad

import "github.com/azrtydxb/momo"

type Doc struct {
	momo.Model `momo:"table:docs"`
	ID         int64   `momo:"pk,identity"`
	Images     []Image `momo:"rel:has_many,fk:OwnerID,polytype:owner_type"`                        // BAD-1
	Tags       []Tag   `momo:"rel:many2many,join:doc_tags,polytype:owner_type,polyvalue:document"` // BAD-2
	Notes      []Note  `momo:"rel:has_many,fk:OwnerID,polytype:owner_type,polyvalue:document"`     // BAD-3
}

type Image struct {
	momo.Model `momo:"table:images"`
	ID         int64 `momo:"pk,identity"`
	OwnerID    int64 `momo:"column:owner_id"`
}

type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64 `momo:"pk,identity"`
}

type Note struct {
	momo.Model `momo:"table:notes"`
	ID         int64 `momo:"pk,identity"`
	OwnerID    int64 `momo:"column:owner_id"`
}
