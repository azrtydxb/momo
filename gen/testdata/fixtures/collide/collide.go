package collide

import "github.com/azrtydxb/momo"

type Table struct {
	momo.Model `momo:"table:tables"`
	ID         int64
	Table      string
	Type       string `momo:"column:type"`
	Select     string `momo:"column:select"`
	Order      int64
}

type Type2 struct {
	momo.Model `momo:"table:type2"`
	ID         int64
	UserID     int64 `momo:"column:user_id"`
	UserId     int64
}
