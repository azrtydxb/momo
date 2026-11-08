// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/jackc/pgx/v5/pgconn"
)

// A unique violation never shows the offending value in Error(), keeps it
// reachable through Detail() and errors.As. Fails if the classifier copies the
// PostgreSQL key detail into the rendered text or drops it from Detail.
func TestErrorRedaction(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL key detail only")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := db.Driver().Exec(ctx, "CREATE UNIQUE INDEX documents_title_uq ON documents (title)", nil); err != nil {
			t.Fatal(err)
		}
		const ins = "INSERT INTO documents (tenant_id, title) VALUES (1, 'secret@example.com')"
		if _, err := momo.Exec(ctx, db, ins); err != nil {
			t.Fatal(err)
		}
		_, err := momo.Exec(ctx, db, ins)
		if !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("err = %v, want ErrUniqueViolation", err)
		}
		if strings.Contains(err.Error(), "secret@example.com") {
			t.Fatalf("Error() leaked the value: %q", err.Error())
		}
		var me *momo.Error
		if !errors.As(err, &me) {
			t.Fatal("not a *momo.Error")
		}
		if !strings.Contains(me.Detail(), "secret@example.com") {
			t.Fatalf("Detail() lacks the raw detail: %q", me.Detail())
		}
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Detail == "" {
			t.Fatal("raw PgError not reachable with errors.As")
		}
	})
}
