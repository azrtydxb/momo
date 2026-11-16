// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Catches: the default refusal of a key assignment weakening, so a Set of a
// key column (single or composite) renders without the explicit opt-in.
func TestUpdateKeyAssignRefusedWithoutAllowKeyChange(t *testing.T) {
	pg := bulkDB(dialect.For(dialect.Postgres))
	for name, set := range map[string]momo.Assignment{"region": bulkRegion.Set("us"), "sku": bulkSKU.Set(9)} {
		_, err := momo.Update[bulkStock](pg).Where(bulkQty.Eq(1)).Set(set).SQL()
		if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "primary key") {
			t.Errorf("Set(%s) without opt-in err = %v, want ErrValidation naming the primary key", name, err)
		}
	}
}

// Catches: the opt-in not rendering the key in SET, or rendering it on only
// one dialect.
func TestUpdateAllowKeyChangeRenders(t *testing.T) {
	where := []momo.Predicate{bulkRegion.Eq("eu"), bulkSKU.Eq(1)}
	c, err := momo.Update[bulkStock](bulkDB(dialect.For(dialect.Postgres))).
		Set(bulkRegion.Set("us"), bulkSKU.Set(2)).Where(where...).AllowKeyChange().SQL()
	want := `UPDATE "bulk_stock" SET "region" = $1, "sku" = $2 WHERE "bulk_stock"."region" = $3 AND "bulk_stock"."sku" = $4`
	if err != nil || c.SQL != want || len(c.Args) != 4 {
		t.Fatalf("postgres = %q %v, want %q", c.SQL, err, want)
	}
	for _, name := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		c, err := momo.Update[bulkStock](bulkDB(dialect.For(name))).
			Set(bulkRegion.Set("us")).Where(where...).AllowKeyChange().SQL()
		if err != nil || !strings.HasPrefix(c.SQL, "UPDATE ") || len(c.Args) != 3 {
			t.Errorf("%s = %q %v, want a 3-argument UPDATE", name, c.SQL, err)
		}
	}
}

// Catches: the opt-in lifting more than the key refusal: version, an identity
// key, Model, BulkUpdate and an assignment-free statement stay refused.
func TestUpdateAllowKeyChangeStillRefuses(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	ver := momo.NewColumn[int64]("roles", "ver", 0)
	if _, err := momo.Update[updRoles](pg).Set(ver.Set(1)).AllRows().AllowKeyChange().SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("version with opt-in err = %v, want ErrValidation", err)
	}
	id := momo.NewColumn[int64]("roles", "id", momo.FlagPK)
	if _, err := momo.Update[updRoles](pg).Set(id.Set(5)).Where(id.Eq(1)).AllowKeyChange().SQL(); err != nil {
		t.Errorf("key with opt-in err = %v", err)
	}
	if _, err := momo.Update[updRoles](pg).Set(id.Set(5)).AllowKeyChange().SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Errorf("key with opt-in and no Where err = %v, want ErrUnfilteredMutation", err)
	}
	m := updRoles{ID: 1, Ver: 1}
	if _, err := momo.Update[updRoles](pg).Model(&m).Set(id.Set(5)).AllowKeyChange().SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Model with key assignment err = %v, want ErrValidation", err)
	}
	gid := momo.NewColumn[int64]("gens", "id", momo.FlagPK|momo.FlagIdentity)
	if _, err := momo.Update[updGen](pg).Set(gid.Set(5)).Where(gid.Eq(1)).AllowKeyChange().SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("identity key with opt-in err = %v, want ErrValidation", err)
	}
	bulk := momo.BulkUpdateKeys[bulkStock](bulkDB(dialect.For(dialect.Postgres)), []momo.ColumnRef{bulkRegion, bulkSKU},
		[]momo.ColumnRef{bulkQty}, [][]any{{"eu", int64(1), int64(2)}}).AllowKeyChange()
	if _, err := bulk.SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("BulkUpdate with opt-in err = %v, want ErrValidation", err)
	}
}
