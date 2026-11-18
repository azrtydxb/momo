// SPDX-License-Identifier: Apache-2.0

// Package companionhistoryrefused asks for a history that cannot be built.
package companionhistoryrefused

import "github.com/azrtydxb/momo"

// NoKey has no primary key to find the versions of a row by.
type NoKey struct {
	momo.Model `momo:"table:no_keys"`
	Name       string
}

func (n *NoKey) MomoConfig(c *momo.Config) {
	c.History()
}

// Reserved has a column named like a history column.
type Reserved struct {
	momo.Model `momo:"table:reserveds"`
	ID         int64 `momo:"pk"`
	HistoryID  int64
}

func (r *Reserved) MomoConfig(c *momo.Config) {
	c.History()
}

// Owner wants owners_history, which OwnersHistory already is.
type Owner struct {
	momo.Model `momo:"table:owners"`
	ID         int64 `momo:"pk"`
}

func (o *Owner) MomoConfig(c *momo.Config) {
	c.History()
}

type OwnersHistory struct {
	momo.Model `momo:"table:owners_history"`
	ID         int64 `momo:"pk"`
}

// Scoped is in a schema other than the current one.
type Scoped struct {
	momo.Model `momo:"table:scoped,schema:app"`
	ID         int64 `momo:"pk"`
}

func (s *Scoped) MomoConfig(c *momo.Config) {
	c.History()
}

// LongName has a table name whose derived history names PostgreSQL would cut.
type LongName struct {
	momo.Model `momo:"table:a_table_whose_name_is_too_long_for_its_derived_names"`
	ID         int64 `momo:"pk"`
}

func (l *LongName) MomoConfig(c *momo.Config) {
	c.History()
}
