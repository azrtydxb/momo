package crossa

import "github.com/azrtydxb/momo"

type Owner struct {
	momo.Model `momo:"table:owners"`
	ID         int64  `momo:"pk,identity"`
	Name       string `momo:"notnull"`
}
