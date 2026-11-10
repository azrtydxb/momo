// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
)

func TestAnalyticsLockRefusals(t *testing.T) {
	pg := typedDB(dialect.Postgres)
	cur := momo.UnnestValues("currency", []string{"EUR"})
	ser := momo.GenerateSeries("g", "n", 1, 3)
	sub := momo.Select[anKT](pg).From(anDocs).Columns(DocID, DocTitle)
	tenants := momo.TableRef{Name: "tenants"}
	tid := momo.ColumnOf[int64]("tenants", "id")
	cases := []struct {
		name string
		q    momo.SelectQuery[anKT]
	}{
		{"distinct on", momo.Select[anKT](pg).From(anDocs).Columns(DocTenantID, DocTitle).
			DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc()).ForUpdate()},
		{"lateral alias", momo.Select[anKT](pg).From(tenants).Columns(tid, momo.ColumnOf[string]("lo", "title")).
			CrossJoin(momo.Lateral(sub, "lo")).ForUpdate().LockOf("lo")},
		{"unnest alias", momo.Select[anKT](pg).From(momo.Unnest("t", cur)).Columns(cur.Ref("t"), cur.Ref("t")).
			ForUpdate().LockOf("t")},
		{"series alias", momo.Select[anKT](pg).From(ser).Columns(ser.Column(), ser.Column()).
			ForUpdate().LockOf("g")},
	}
	for _, c := range cases {
		if _, err := c.q.SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", c.name, err)
		}
	}
}

// TestLagDefaultMasksSensitiveColumn pins that the default of Lag is hidden
// from hooks when the lagged column is sensitive.
func TestLagDefaultMasksSensitiveColumn(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	_, err := momo.Select[anKT](db).From(anDocs).
		Columns(suite.DocTitle, momo.LagDefault(suite.DocSecret, 1, sensitiveValue).Over(momo.Over().OrderBy(suite.DocID.Asc()))).
		All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertMasked(t, rec, drv)
}

func TestDistinctOnDeduplicates(t *testing.T) {
	c, err := momo.Select[anKT](typedDB(dialect.Postgres)).From(anDocs).Columns(DocTenantID, DocTitle).
		DistinctOn(DocTenantID, DocTenantID).OrderBy(DocTenantID.Asc(), DocID.Asc()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT DISTINCT ON ("documents"."tenant_id") "documents"."tenant_id", "documents"."title" FROM "documents" ORDER BY "documents"."tenant_id" ASC, "documents"."id" ASC`
	if c.SQL != want {
		t.Errorf("got %s\nwant %s", c.SQL, want)
	}
}
