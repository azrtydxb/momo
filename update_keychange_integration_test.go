// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// kcTeam is a table with a user-chosen text key; kcMember references it with
// ON UPDATE CASCADE.
type kcTeam struct {
	ID   string
	Name string
}

type kcMember struct {
	ID     int64
	TeamID string
}

var (
	kcTeamID     = momo.NewColumn[string]("kc_team", "id", momo.FlagPK)
	kcTeamName   = momo.NewColumn[string]("kc_team", "name", 0)
	kcMemberTeam = momo.NewColumn[string]("kc_member", "team_id", 0)
)

var kcTeamMeta = momo.Meta[kcTeam]{
	Table:      "kc_team",
	Columns:    []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "name"}},
	Ptr:        func(m *kcTeam, i int) any { return [...]any{&m.ID, &m.Name}[i] },
	Value:      func(m *kcTeam, i int) any { return [...]any{m.ID, m.Name}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (*kcTeam) MomoMeta() *momo.Meta[kcTeam] { return &kcTeamMeta }

var kcMemberMeta = momo.Meta[kcMember]{
	Table:      "kc_member",
	Columns:    []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "team_id"}},
	Ptr:        func(m *kcMember, i int) any { return [...]any{&m.ID, &m.TeamID}[i] },
	Value:      func(m *kcMember, i int) any { return [...]any{m.ID, m.TeamID}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (*kcMember) MomoMeta() *momo.Meta[kcMember] { return &kcMemberMeta }

func createKeyChangeTables(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	drop := func() {
		_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS kc_member")
		_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS kc_team")
	}
	drop()
	for _, stmt := range []string{
		"CREATE TABLE kc_team (id varchar(64) NOT NULL PRIMARY KEY, name varchar(64) NOT NULL)",
		"CREATE TABLE kc_member (id bigint NOT NULL PRIMARY KEY, team_id varchar(64) NOT NULL, FOREIGN KEY (team_id) REFERENCES kc_team (id) ON UPDATE CASCADE)",
		"INSERT INTO kc_team (id, name) VALUES ('alpha', 'Alpha'), ('gamma', 'Gamma')",
		"INSERT INTO kc_member (id, team_id) VALUES (1, 'alpha'), (2, 'alpha'), (3, 'gamma')",
	} {
		if _, err := momo.Exec(ctx, db, stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(drop)
}

// Catches: the opt-in being a no-op (the refusal still firing), the new key not
// reaching the row, a read by the new key missing it or the old key still
// finding it, a duplicate key not mapping to ErrUniqueViolation (or leaving a
// half-changed row), an ON UPDATE CASCADE child not following the key, a
// RETURNING list carrying the old key, and a composite key changing only one
// column.
func TestUpdateAllowKeyChangeIntegration(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		createKeyChangeTables(t, db)
		createBulkStock(t, db)
		ctx := context.Background()

		// The default refusal stays, and nothing changed.
		_, err := momo.Update[kcTeam](db).Set(kcTeamID.Set("beta")).Where(kcTeamID.Eq("alpha")).Exec(ctx)
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("key change without opt-in = %v, want ErrValidation", err)
		}

		// A key change runs as written; the row is read back by the new key.
		res, err := momo.Update[kcTeam](db).Set(kcTeamID.Set("beta")).Where(kcTeamID.Eq("alpha")).AllowKeyChange().Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("key change = %+v, %v; want 1 row", res, err)
		}
		got, err := momo.Select[kcTeam](db).Where(kcTeamID.Eq("beta")).One(ctx)
		if err != nil || got.Name != "Alpha" {
			t.Fatalf("read by the new key = %+v, %v; want Alpha", got, err)
		}
		if _, err := momo.Select[kcTeam](db).Where(kcTeamID.Eq("alpha")).One(ctx); !errors.Is(err, momo.ErrNotFound) {
			t.Fatalf("read by the old key = %v, want ErrNotFound", err)
		}

		// The children declared ON UPDATE CASCADE follow the new key.
		if n, err := momo.Select[kcMember](db).Where(kcMemberTeam.Eq("beta")).Count(ctx); err != nil || n != 2 {
			t.Fatalf("members of beta = %d, %v; want 2", n, err)
		}
		if n, err := momo.Select[kcMember](db).Where(kcMemberTeam.Eq("gamma")).Count(ctx); err != nil || n != 1 {
			t.Fatalf("members of gamma = %d, %v; want 1", n, err)
		}

		// A key held by another row is a unique violation and changes nothing.
		_, err = momo.Update[kcTeam](db).Set(kcTeamID.Set("gamma")).Where(kcTeamID.Eq("beta")).AllowKeyChange().Exec(ctx)
		if !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("duplicate key = %v, want ErrUniqueViolation", err)
		}
		if n, err := momo.Select[kcTeam](db).Where(kcTeamID.Eq("beta")).Count(ctx); err != nil || n != 1 {
			t.Fatalf("beta after the refused change = %d, %v; want 1", n, err)
		}

		// RETURNING reads the row under its new key where the dialect has it.
		rows, err := momo.Update[kcTeam](db).Set(kcTeamID.Set("delta")).Where(kcTeamID.Eq("beta")).AllowKeyChange().ReturningAll().Rows(ctx)
		if db.Dialect().Name == dialect.MySQL {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("UPDATE RETURNING on the MySQL family = %v, want ErrUnsupported", err)
			}
			if _, err := momo.Update[kcTeam](db).Set(kcTeamID.Set("delta")).Where(kcTeamID.Eq("beta")).AllowKeyChange().Exec(ctx); err != nil {
				t.Fatal(err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			// Closed before the next statement, which may need the connection.
			func() {
				defer rows.Close()
				var back []kcTeam
				for rows.Next() {
					var m kcTeam
					if err := rows.ScanModel(&m); err != nil {
						t.Fatal(err)
					}
					back = append(back, m)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				if len(back) != 1 || back[0].ID != "delta" || back[0].Name != "Alpha" {
					t.Fatalf("RETURNING = %+v, want the row under delta", back)
				}
			}()
		}
		if n, err := momo.Select[kcMember](db).Where(kcMemberTeam.Eq("delta")).Count(ctx); err != nil || n != 2 {
			t.Fatalf("members of delta = %d, %v; want 2", n, err)
		}

		// A composite key: each key column may change, and a clash is a unique
		// violation.
		res, err = momo.Update[bulkStock](db).Set(bulkSKU.Set(3)).Where(bulkRegion.Eq("eu"), bulkSKU.Eq(1)).AllowKeyChange().Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("composite key change = %+v, %v; want 1 row", res, err)
		}
		stock := readStock(t, db)
		wantStock(t, stock, "eu3", 10, kcStr("a"))
		if _, ok := stock["eu1"]; ok {
			t.Errorf("row eu1 still present after its key changed")
		}
		_, err = momo.Update[bulkStock](db).Set(bulkRegion.Set("us"), bulkSKU.Set(1)).Where(bulkRegion.Eq("eu"), bulkSKU.Eq(2)).AllowKeyChange().Exec(ctx)
		if !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("composite duplicate = %v, want ErrUniqueViolation", err)
		}
	})
}

// kcStr returns a pointer to s.
func kcStr(s string) *string { return &s }
