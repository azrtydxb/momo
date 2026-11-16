// SPDX-License-Identifier: Apache-2.0

package checkinbadschema

import "github.com/azrtydxb/momo"

type Typed struct {
	momo.Model `momo:"table:typeds"`
	ID         int64  `momo:"pk,identity"`
	Status     string `momo:"enum:typed_status"`
}

func (s *Typed) MomoConfig(c *momo.Config) {
	c.Enum("typed_status", "a", "b")
	c.CheckIn(&s.Status, "a")
}

type Priced struct {
	momo.Model `momo:"table:priceds"`
	ID         int64  `momo:"pk,identity"`
	Price      string `momo:"type:numeric(10,2)"`
}

func (p *Priced) MomoConfig(c *momo.Config) {
	c.CheckIn(&p.Price, "1")
}

type Slashed struct {
	momo.Model `momo:"table:slasheds"`
	ID         int64 `momo:"pk,identity"`
	Kind       string
}

func (s *Slashed) MomoConfig(c *momo.Config) {
	c.CheckIn(&s.Kind, "a\\b")
}
