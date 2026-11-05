// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

type compDoc struct {
	Model
	ID        int64
	Title     string
	Embedding Vector32
}

// Vector32 stands in for a vector field in this companion-only test.
type Vector32 []float32

func (d *compDoc) MomoConfig(c *Config) {
	c.Index("docs_title_idx", &d.Title).Where("title <> ''").Method("btree").Desc(&d.Title).Nulls(&d.Title, "last")
	c.IndexExpr("docs_lower_title", "lower(title)").Concurrently()
	c.Unique("docs_title_key", &d.Title).With("fillfactor", "90").Opclass(&d.Title, "text_ops")
	c.Check("docs_title_nonempty", "length(title) > 0")
	c.Enum("doc_status", "draft", "published")
	c.Default(&d.Title, "'untitled'")
	c.Comment(&d.Title, "a title, with a comma")
	c.Collate(&d.Title, "C")
}

func TestCompanionDSLCompiles(t *testing.T) {
	var d compDoc
	d.MomoConfig(&Config{})
}

func (d *compDoc) managed(c *Config) {
	c.View(schema.View{Name: "v", Query: "select 1"})
	c.Function(schema.Function{Name: "f", Language: "sql"})
	c.Trigger(schema.Trigger{Table: "docs", Name: "t"})
	c.Policy(schema.Policy{Table: "docs", Name: "p", Command: "ALL"})
	c.RLS(schema.RLS{Enabled: true})
	c.Partition(schema.Partition{Strategy: "range", Key: "RANGE (id)"})
	c.PartitionOf(schema.PartitionOf{Parent: "docs"})
}

func TestCompanionManagedObjectsCompile(t *testing.T) {
	var d compDoc
	d.managed(&Config{})
}
