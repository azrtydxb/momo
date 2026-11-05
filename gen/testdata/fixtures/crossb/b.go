package crossb

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/gen/testdata/fixtures/crossa"
)

type Item struct {
	momo.Model `momo:"table:items"`
	ID         int64         `momo:"pk,identity"`
	OwnerID    int64         `momo:"index"`
	Owner      *crossa.Owner `momo:"belongs_to,fk:OwnerID"`
}
