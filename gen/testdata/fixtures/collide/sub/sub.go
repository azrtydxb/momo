package sub

import "github.com/azrtydxb/momo"

type Sub struct {
	momo.Model `momo:"table:subs"`
	ID         int64 `momo:"pk"`
}
