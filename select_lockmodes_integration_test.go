// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestNoKeyUpdateKeyShareConflicts runs the PostgreSQL row-lock matrix for the
// two weaker modes: FOR NO KEY UPDATE does not block FOR KEY SHARE (the lock
// a foreign-key insert takes on the parent row) but does block FOR UPDATE,
// FOR SHARE and another FOR NO KEY UPDATE; FOR KEY SHARE blocks only
// FOR UPDATE.
func TestNoKeyUpdateKeyShareConflicts(t *testing.T) {
	ctx := context.Background()
	db := testopen.Postgres(t)
	caps := suite.CapsOf(db)
	suite.CreateDocuments(t, db, caps)
	if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", "a"); err != nil {
		t.Fatal(err)
	}

	lock := func(q momo.Querier, mode string) error {
		s := momo.Select[suite.Document](q).Where(suite.DocTitle.Eq("a"))
		switch mode {
		case "update":
			s = s.ForUpdate("NOWAIT")
		case "share":
			s = s.ForShare("NOWAIT")
		case "nokey":
			s = s.ForNoKeyUpdate("NOWAIT")
		case "keyshare":
			s = s.ForKeyShare("NOWAIT")
		}
		rows, err := s.All(ctx)
		if err == nil && len(rows) != 1 {
			t.Fatalf("%s locked %d rows, want 1", mode, len(rows))
		}
		return err
	}

	matrix := []struct {
		holder, rival string
		blocked       bool
	}{
		{"nokey", "keyshare", false},
		{"nokey", "nokey", true},
		{"nokey", "share", true},
		{"nokey", "update", true},
		{"keyshare", "keyshare", false},
		{"keyshare", "nokey", false},
		{"keyshare", "share", false},
		{"keyshare", "update", true},
	}
	for _, m := range matrix {
		holder, err := db.Begin(ctx, momo.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		rival, err := db.Begin(ctx, momo.TxOptions{})
		if err != nil {
			_ = holder.Rollback(ctx)
			t.Fatal(err)
		}
		if err := lock(holder, m.holder); err != nil {
			t.Fatalf("%s: holder lock: %v", m.holder, err)
		}
		err = lock(rival, m.rival)
		if m.blocked && !errors.Is(err, momo.ErrBusy) {
			t.Errorf("%s then %s: err = %v, want ErrBusy", m.holder, m.rival, err)
		}
		if !m.blocked && err != nil {
			t.Errorf("%s then %s: err = %v, want no conflict", m.holder, m.rival, err)
		}
		_ = rival.Rollback(ctx)
		_ = holder.Rollback(ctx)
	}
}
