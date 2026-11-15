// SPDX-License-Identifier: Apache-2.0

package fulltextbad

import "github.com/azrtydxb/momo"

type Counter struct {
	momo.Model `momo:"table:counters"`
	ID         int64 `momo:"pk,identity"`
	Views      int64 `momo:"search"`
}

type Tagged struct {
	momo.Model `momo:"table:taggeds"`
	ID         int64    `momo:"pk,identity"`
	Labels     []string `momo:"search"`
}

type HostileConfig struct {
	momo.Model `momo:"table:hostile_configs"`
	ID         int64 `momo:"pk,identity"`
	Title      string
}

func (h *HostileConfig) MomoConfig(c *momo.Config) {
	c.FullText("hostile_configs_search", &h.Title).Config("english'); DROP TABLE x;--")
}

type HostileTokenizer struct {
	momo.Model `momo:"table:hostile_tokenizers"`
	ID         int64 `momo:"pk,identity"`
	Title      string
}

func (h *HostileTokenizer) MomoConfig(c *momo.Config) {
	c.FullText("hostile_tokenizers_search", &h.Title).Tokenizer("unicode61'); DROP TABLE x;--")
}

type HostileParser struct {
	momo.Model `momo:"table:hostile_parsers"`
	ID         int64 `momo:"pk,identity"`
	Title      string
}

func (h *HostileParser) MomoConfig(c *momo.Config) {
	c.FullText("hostile_parsers_search", &h.Title).Parser("ngram; x")
}

type DuplicateOption struct {
	momo.Model `momo:"table:duplicate_options"`
	ID         int64 `momo:"pk,identity"`
	Title      string
}

func (d *DuplicateOption) MomoConfig(c *momo.Config) {
	c.FullText("duplicate_options_search", &d.Title).Config("english").Config("simple")
}

type WeightMismatch struct {
	momo.Model `momo:"table:weight_mismatches"`
	ID         int64 `momo:"pk,identity"`
	Title      string
	Body       string
}

func (w *WeightMismatch) MomoConfig(c *momo.Config) {
	c.FullText("weight_mismatches_search", &w.Title, &w.Body).Weights("A")
}

type Unresolved struct {
	momo.Model `momo:"table:unresolveds"`
	ID         int64 `momo:"pk,identity"`
	Title      string
}

func (u *Unresolved) MomoConfig(c *momo.Config) {
	var other Counter
	c.FullText("unresolveds_search", &other.Views)
}

type Both struct {
	momo.Model `momo:"table:boths"`
	ID         int64  `momo:"pk,identity"`
	Title      string `momo:"search"`
	Body       string
}

func (b *Both) MomoConfig(c *momo.Config) {
	c.FullText("boths_search", &b.Title, &b.Body)
}
