package brokenpkg

import "github.com/azrtydxb/momo"

type Thing struct {
	momo.Model
	ID   int64
	Name Undefined
}
