// SPDX-License-Identifier: Apache-2.0

package rel

import "github.com/azrtydxb/momo"

type Author struct {
	momo.Model `momo:"table:authors"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"notnull"`
	Posts      []Post `momo:"has_many,fk:AuthorID"`
}

type Post struct {
	momo.Model `momo:"table:posts"`
	ID         int64   `momo:"pk,identity"`
	AuthorID   int64   `momo:"column:author_id,notnull"`
	Author     *Author `momo:"belongs_to,fk:AuthorID,on_delete:cascade,on_update:restrict"`
}
