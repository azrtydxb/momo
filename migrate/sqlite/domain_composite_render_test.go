// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a domain or composite type change rendered as empty or invalid SQL
// on SQLite instead of being refused with ErrUnsupported.
func TestSQLiteRenderRefusesDomainsAndComposites(t *testing.T) {
	d := schema.Domain{Name: "email", Type: "text"}
	c := schema.Composite{Name: "address", Attributes: []schema.CompositeAttr{{Name: "street", Type: "text"}}}
	for _, ch := range []migrate.Change{
		{Kind: migrate.CreateDomain, Name: "email", After: &d},
		{Kind: migrate.AlterDomain, Name: "email", Before: &d, After: &d},
		{Kind: migrate.RecreateDomain, Name: "email", Before: &d, After: &d},
		{Kind: migrate.DropDomain, Name: "email", Before: &d},
		{Kind: migrate.CreateComposite, Name: "address", After: &c},
		{Kind: migrate.AlterComposite, Name: "address", Before: &c, After: &c},
		{Kind: migrate.DropComposite, Name: "address", Before: &c},
	} {
		stmts, err := Dialect{}.Render([]migrate.Change{ch})
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: want ErrUnsupported, got stmts %+v err %v", ch.Kind, stmts, err)
		}
	}
}
