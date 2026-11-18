// SPDX-License-Identifier: Apache-2.0

// Package companiondeferrablefk has foreign keys that reference deferrable
// keys, which PostgreSQL refuses.
package companiondeferrablefk

import "github.com/azrtydxb/momo"

// Parent has a deferrable primary key that Child references through belongs_to.
type Parent struct {
	momo.Model `momo:"table:parents"`
	ID         int64 `momo:"pk"`

	Children []Child `momo:"has_many,fk:ParentID"`
}

func (p *Parent) MomoConfig(c *momo.Config) {
	c.PrimaryKeyDeferrable()
}

// Slot has a deferrable unique constraint that Child references through
// Config.ForeignKey.
type Slot struct {
	momo.Model `momo:"table:slots"`
	ID         int64 `momo:"pk"`
	Code       string
}

func (s *Slot) MomoConfig(c *momo.Config) {
	c.Unique("slots_code_key", &s.Code).Deferrable()
}

type Child struct {
	momo.Model `momo:"table:children"`
	ID         int64 `momo:"pk"`
	ParentID   int64
	Code       string

	Parent *Parent `momo:"belongs_to,fk:ParentID"`
}

func (m *Child) MomoConfig(c *momo.Config) {
	c.ForeignKey("children_code_fkey", []any{&m.Code}, "slots", []string{"code"})
}
