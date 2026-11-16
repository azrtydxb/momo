// SPDX-License-Identifier: Apache-2.0

package checkinbad

import "github.com/azrtydxb/momo"

type Repeated struct {
	momo.Model `momo:"table:repeateds"`
	ID         int64 `momo:"pk,identity"`
	Kind       string
}

func (r *Repeated) MomoConfig(c *momo.Config) {
	c.CheckIn(&r.Kind, "a", "b", "a")
}

type Empty struct {
	momo.Model `momo:"table:empties"`
	ID         int64 `momo:"pk,identity"`
	Kind       string
}

func (e *Empty) MomoConfig(c *momo.Config) {
	c.CheckIn(&e.Kind)
}

type NotString struct {
	momo.Model `momo:"table:not_strings"`
	ID         int64 `momo:"pk,identity"`
	Count      int64
}

func (n *NotString) MomoConfig(c *momo.Config) {
	c.CheckIn(&n.Count, "1", "2")
}

type Clash struct {
	momo.Model `momo:"table:clashes"`
	ID         int64  `momo:"pk,identity"`
	Kind       string `momo:"check:kind <> ''"`
	Label      string
}

func (k *Clash) MomoConfig(c *momo.Config) {
	c.CheckIn(&k.Kind, "a")
	c.Check("clashes_label_in", "label <> ''")
	c.CheckIn(&k.Label, "a").Name("clashes_label_in")
}

type Variable struct {
	momo.Model `momo:"table:variables"`
	ID         int64 `momo:"pk,identity"`
	Kind       string
}

func (v *Variable) MomoConfig(c *momo.Config) {
	values := []string{"a", "b"}
	c.CheckIn(&v.Kind, values...)
}
