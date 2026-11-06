// SPDX-License-Identifier: Apache-2.0

package companionbad

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

var idxVar = "hostile_var_idx"

type Loop struct {
	momo.Model `momo:"table:loops"`
	ID         int64
	Name       string
}

func (l *Loop) MomoConfig(c *momo.Config) {
	for _, n := range []string{"a", "b"} {
		c.Index(n, &l.Name)
	}
	if l.Name == "" {
		c.Index("loops_name_idx", &l.Name)
	}
	c.Index(idxVar, &l.Name)
	c.Check("loops_a"+"_check", "true")
}

type Refs struct {
	momo.Model `momo:"table:refs"`
	ID         int64
	Name       string
	Hidden     string `momo:"-"`
	Owner      Loop
}

func (r *Refs) MomoConfig(c *momo.Config) {
	c.Index("refs_hidden_idx", &r.Hidden)
	c.Index("refs_model_idx", &r.Model)
	c.Index("refs_owner_idx", &r.Owner.Name)
	c.Index("refs_other_idx", &(&Loop{}).Name)
	c.Index("refs_chain_idx", &r.Name).Desc(&r.ID)
}

type Anon struct {
	momo.Model `momo:"table:anons"`
	ID         int64
	Name       string
}

func (*Anon) MomoConfig(c *momo.Config) {
	c.Index("anons_name_idx", &(&Anon{}).Name)
}

type Dups struct {
	momo.Model `momo:"table:dups"`
	ID         int64
	Slug       string `momo:"index"`
	Name       string
}

func (d *Dups) MomoConfig(c *momo.Config) {
	c.Index("dups_name_idx", &d.Name)
	c.Unique("dups_name_idx", &d.Name)
	c.Index("dups_slug_idx", &d.Slug)
	c.Default(&d.Name, "a")
	c.Default(&d.Name, "b")
}

type BadSig struct {
	momo.Model `momo:"table:badsigs"`
	ID         int64
}

func (b *BadSig) MomoConfig(c *schema.Table) {}

type Struct struct {
	momo.Model `momo:"table:structs"`
	ID         int64
}

func (s *Struct) MomoConfig(c *momo.Config) {
	name := "v"
	c.View(schema.View{Name: name, Query: "select 1"})
	c.View(schema.View{"", "v", false, "select 1", nil, nil})
	c.View(schema.View{Name: "n", Query: "select 1", Indexes: []schema.Index{}})
	c.RLS(schema.RLS{Enabled: true, Forced: true})
	c.Policy(schema.Policy{Name: "p" + "q"})
}
