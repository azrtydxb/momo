// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// unitTally is a projection fixture: it has no table of its own.
type unitTally struct{ N int64 }

var unitTallyMeta = momo.Meta[unitTally]{
	Columns:    []momo.ColumnMeta{{Name: "n"}},
	Ptr:        func(m *unitTally, _ int) any { return &m.N },
	Value:      func(m *unitTally, _ int) any { return m.N },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
}

func (*unitTally) MomoMeta() *momo.Meta[unitTally] { return &unitTallyMeta }

func TestAfterNeedsValues(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	_, err := momo.Select[unitDocument](pg).OrderBy(DocTitle.Asc()).After().SQL()
	if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "After needs at least one value") {
		t.Fatalf("After() = %v, want ErrValidation naming the missing values", err)
	}
}

func TestCountAndExistsIgnoreCursor(t *testing.T) {
	d := fakedriver.New(dialect.Postgres)
	pg := momo.New(d)
	q := momo.Select[unitDocument](pg).OrderBy(DocTitle.Asc()).After("m", int64(3))
	if _, err := q.Count(context.Background()); err != nil {
		t.Fatalf("Count with a cursor: %v", err)
	}
	if _, err := q.Exists(context.Background()); err != nil {
		t.Fatalf("Exists with a cursor: %v", err)
	}
	for _, c := range d.Calls() {
		if strings.Contains(c.SQL, `"title" >`) || strings.Contains(c.SQL, "ORDER BY") {
			t.Errorf("count/exists SQL carries the page cursor: %s", c.SQL)
		}
	}
	if len(d.Calls()) != 2 {
		t.Fatalf("calls = %d, want 2", len(d.Calls()))
	}
}

func TestNegativeLimitAndOffset(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	if _, err := momo.Select[unitDocument](pg).Limit(-5).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Limit(-5) = %v, want ErrValidation", err)
	}
	if _, err := momo.Select[unitDocument](pg).Offset(-1).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Offset(-1) = %v, want ErrValidation", err)
	}
	if _, err := momo.Select[unitDocument](pg).Limit(0).Offset(0).SQL(); err != nil {
		t.Errorf("zero Limit/Offset = %v, want nil", err)
	}
}

func TestKeysetTieBreakerNeedsOwnTable(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	other := momo.ColumnOf[int64]("other", "id")
	c, err := momo.Select[unitDocument](pg).CrossJoin(momo.TableRef{Name: "other"}).OrderBy(other.Asc()).After(int64(1), int64(2)).SQL()
	if err != nil {
		t.Fatalf("ordering by another table's id must still add the primary key: %v", err)
	}
	if !strings.Contains(c.SQL, `ORDER BY "other"."id" ASC, "documents"."id" ASC`) {
		t.Fatalf("primary key tie-breaker missing: %s", c.SQL)
	}
	// Ordering by the model's own id needs no extra key.
	if _, err := momo.Select[unitDocument](pg).OrderBy(DocID.Asc()).After(int64(1)).SQL(); err != nil {
		t.Fatalf("own primary key: %v", err)
	}
}

func TestBuildErrorsAreValidationErrors(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	cases := map[string]error{
		"raw placeholders": func() error {
			_, err := momo.Select[unitDocument](pg).Where(momo.SQL("a = ?")).SQL()
			return err
		}(),
		"raw surplus args": func() error {
			_, err := momo.Select[unitDocument](pg).Where(momo.SQL("a = 1", 5)).SQL()
			return err
		}(),
		"column count": func() error {
			_, err := momo.Select[unitDocument](pg).Columns(DocID).SQL()
			return err
		}(),
		"projection without columns": func() error {
			_, err := momo.Select[unitTally](pg).SQL()
			return err
		}(),
	}
	for name, err := range cases {
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: %v, want ErrValidation", name, err)
		}
	}
}

func TestSoftDeleteFilterWithColumns(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	all := []momo.Expr{DocID, DocTenantID, DocTitle, DocPublished, DocDeletedAt}
	c, err := momo.Select[unitDocument](db).Columns(all...).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL`
	if c.SQL != want {
		t.Fatalf("soft-delete filter lost when columns are restricted:\n got %s\nwant %s", c.SQL, want)
	}
}

func TestTableRefSchemaQualified(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[unitDocument](db).From(momo.TableRef{Name: "documents", Schema: "tenant"}).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "tenant"."documents" WHERE "documents"."deleted_at" IS NULL`; c.SQL != want {
		t.Fatalf("got %s\nwant %s", c.SQL, want)
	}
}
