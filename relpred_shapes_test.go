// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/region"
	"github.com/azrtydxb/momo/internal/relfixture/store"
)

// TestRelationPredicateShapes covers self-referencing, composite-key and
// polymorphic descriptors, which the basic tests do not reach.
func TestRelationPredicateShapes(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		// Self-reference: the outer table is aliased and passed to HasFrom.
		bosses := momo.Select[models.Employee, *models.Employee](db).
			From(momo.TableRef{Name: "employees", Alias: "e"}).
			Where(employee.Reports.HasFrom("e")).OrderBy(employee.ID.As("e").Asc())
		c, err := bosses.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.SQL, `"momo_r"."manager_id" = "e"."id"`) {
			t.Fatalf("self-reference must correlate on the outer alias: %s", c.SQL)
		}
		emps, err := bosses.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(emps) != 1 || emps[0].ID != 1 {
			t.Fatalf("employees with reports: %+v", emps)
		}

		// Composite key: both columns correlate, so region (1,EU) matches only
		// its own store rows, not every store of tenant 1.
		regs, err := momo.Select[models.Region, *models.Region](db).
			Where(region.Stores.Has(store.ID.As("momo_r").Eq(2))).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(regs) != 1 || regs[0].Code != "US" {
			t.Fatalf("regions with store 2: %+v", regs)
		}
		regs, err = momo.Select[models.Region, *models.Region](db).
			Where(region.Stores.Has(store.ID.As("momo_r").Eq(3))).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(regs) != 0 {
			t.Fatalf("store 3 has a NULL region and belongs to no region: %+v", regs)
		}

		// Polymorphic: an author-owned image with owner_id 2 must not make
		// document 2 look like it has images.
		if _, err := momo.Exec(ctx, db, `INSERT INTO images (id, owner_type, owner_id, url) VALUES (?, ?, ?, ?)`, int64(3), "author", int64(2), "u3"); err != nil {
			t.Fatal(err)
		}
		withImages, err := momo.Select[models.Document, *models.Document](db).
			Where(document.Images.Has()).OrderBy(document.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(withImages)) != "[1]" {
			t.Fatalf("documents with images: %v", docIDs(withImages))
		}

		// Traversal over a composite key.
		stores, err := region.Stores.From(momo.Select[models.Region, *models.Region](db).
			Where(region.Code.Eq("EU"))).OrderBy(store.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, s := range stores {
			ids = append(ids, s.ID)
		}
		if fmt.Sprint(ids) != "[1 4]" {
			t.Fatalf("stores of EU regions: %v", ids)
		}
	})
}
