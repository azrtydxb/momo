// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
)

// idDriver is a MySQL fake driver that reports a generated key, so a single
// Model insert takes the ExecInsertID path.
type idDriver struct{ *fakedriver.Driver }

func (d idDriver) ExecInsertID(ctx context.Context, sql string, args []any) (int64, int64, error) {
	n, err := d.Exec(ctx, sql, args)
	return n, 42, err
}

// Catches: the generated-key insert path handing hooks the real value of a
// sensitive column because it bypassed the masking of the plain exec path.
func TestMySQLGeneratedKeyInsertMasksSensitiveArgs(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.MySQL)
	db := momo.New(idDriver{drv}, momo.WithHook(rec), momo.WithArgLogging())
	doc := suite.Document{TenantID: 1, Title: "t", Secret: sensitiveValue}
	res, err := momo.Insert[suite.Document](db).Model(&doc).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID != 42 || res.InsertID != 42 {
		t.Fatalf("generated key not written back: model %d, result %d", doc.ID, res.InsertID)
	}
	assertMasked(t, rec, drv)
}
