// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// sdUnix is a hand-written model whose soft-delete marker is a nullable Unix
// integer (table docs_unix).
type sdUnix struct {
	ID          int64
	TenantID    int64
	Title       string
	DeletedUnix *int64
}

var (
	sdUnixID    = momo.NewColumn[int64]("docs_unix", "id", momo.FlagPK)
	sdUnixTitle = momo.NewColumn[string]("docs_unix", "title", 0)
)

var sdUnixMeta = momo.Meta[sdUnix]{
	Table: "docs_unix",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
		{Name: "tenant_id"},
		{Name: "title"},
		{Name: "deleted_unix", Flags: momo.FlagSoftDelete | momo.FlagNullable},
	},
	Ptr:            func(m *sdUnix, i int) any { return [...]any{&m.ID, &m.TenantID, &m.Title, &m.DeletedUnix}[i] },
	Value:          func(m *sdUnix, i int) any { return [...]any{m.ID, m.TenantID, m.Title, m.DeletedUnix}[i] },
	Identity:       0,
	Version:        -1,
	SoftDelete:     3,
	SoftDeleteKind: momo.SoftDeleteUnix,
	CreatedAt:      -1,
	UpdatedAt:      -1,
	PK:             []int{0},
}

func (*sdUnix) MomoMeta() *momo.Meta[sdUnix] { return &sdUnixMeta }

// sdBool is a hand-written model whose soft-delete marker is a boolean
// (table docs_bool).
type sdBool struct {
	ID       int64
	TenantID int64
	Title    string
	Deleted  bool
}

var sdBoolMeta = momo.Meta[sdBool]{
	Table: "docs_bool",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
		{Name: "tenant_id"},
		{Name: "title"},
		{Name: "deleted", Flags: momo.FlagSoftDelete},
	},
	Ptr:            func(m *sdBool, i int) any { return [...]any{&m.ID, &m.TenantID, &m.Title, &m.Deleted}[i] },
	Value:          func(m *sdBool, i int) any { return [...]any{m.ID, m.TenantID, m.Title, m.Deleted}[i] },
	Identity:       0,
	Version:        -1,
	SoftDelete:     3,
	SoftDeleteKind: momo.SoftDeleteBool,
	CreatedAt:      -1,
	UpdatedAt:      -1,
	PK:             []int{0},
}

func (*sdBool) MomoMeta() *momo.Meta[sdBool] { return &sdBoolMeta }

var sdNow = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func TestSoftDeleteSQLGoldens(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres), clockAt(sdNow))
	lite := momo.New(fakedriver.New(dialect.SQLite), clockAt(sdNow))
	my := momo.New(fakedriver.New(dialect.MySQL), clockAt(sdNow))

	type golden struct {
		name string
		got  func() (momo.Compiled, error)
		sql  string
		args []any
	}
	live := sdNow
	cases := []golden{
		{"pg timestamp", func() (momo.Compiled, error) {
			return momo.SoftDelete[suite.Document](pg).Where(suite.DocTitle.Eq("gone")).SQL()
		}, `UPDATE "documents" SET "deleted_at" = $1 WHERE ("documents"."title" = $2) AND ("documents"."deleted_at" IS NULL)`, []any{live, "gone"}},
		{"sqlite timestamp", func() (momo.Compiled, error) {
			return momo.SoftDelete[suite.Document](lite).Where(suite.DocTitle.Eq("gone")).SQL()
		}, `UPDATE "documents" SET "deleted_at" = ? WHERE ("documents"."title" = ?) AND ("documents"."deleted_at" IS NULL)`, []any{live, "gone"}},
		{"mysql timestamp", func() (momo.Compiled, error) {
			return momo.SoftDelete[suite.Document](my).Where(suite.DocTitle.Eq("gone")).SQL()
		}, "UPDATE `documents` SET `deleted_at` = ? WHERE (`documents`.`title` = ?) AND (`documents`.`deleted_at` IS NULL)", []any{live, "gone"}},
		{"pg unix", func() (momo.Compiled, error) {
			return momo.SoftDelete[sdUnix](pg).Where(sdUnixTitle.Eq("x")).SQL()
		}, `UPDATE "docs_unix" SET "deleted_unix" = $1 WHERE ("docs_unix"."title" = $2) AND ("docs_unix"."deleted_unix" IS NULL)`, []any{sdNow.Unix(), "x"}},
		{"sqlite unix", func() (momo.Compiled, error) {
			return momo.SoftDelete[sdUnix](lite).Where(sdUnixTitle.Eq("x")).SQL()
		}, `UPDATE "docs_unix" SET "deleted_unix" = ? WHERE ("docs_unix"."title" = ?) AND ("docs_unix"."deleted_unix" IS NULL)`, []any{sdNow.Unix(), "x"}},
		{"pg bool", func() (momo.Compiled, error) {
			return momo.SoftDelete[sdBool](pg).Where(momo.And(sdBoolID.Eq(1))).SQL()
		}, `UPDATE "docs_bool" SET "deleted" = $1 WHERE ("docs_bool"."id" = $2) AND ("docs_bool"."deleted" = FALSE)`, []any{true, int64(1)}},
		{"sqlite bool", func() (momo.Compiled, error) {
			return momo.SoftDelete[sdBool](lite).Where(sdBoolID.Eq(1)).SQL()
		}, `UPDATE "docs_bool" SET "deleted" = ? WHERE ("docs_bool"."id" = ?) AND ("docs_bool"."deleted" = FALSE)`, []any{true, int64(1)}},
		{"mysql bool", func() (momo.Compiled, error) {
			return momo.SoftDelete[sdBool](my).Where(sdBoolID.Eq(1)).SQL()
		}, "UPDATE `docs_bool` SET `deleted` = ? WHERE (`docs_bool`.`id` = ?) AND (`docs_bool`.`deleted` = FALSE)", []any{true, int64(1)}},
		{"pg force delete", func() (momo.Compiled, error) {
			return momo.ForceDelete[suite.Document](pg).Where(suite.DocTitle.Eq("gone")).SQL()
		}, `DELETE FROM "documents" WHERE "documents"."title" = $1`, []any{"gone"}},
		{"pg select only deleted timestamp", func() (momo.Compiled, error) {
			return momo.Select[suite.Document](pg).OnlyDeleted().SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."body", "documents"."version", "documents"."deleted_at", "documents"."created_at", "documents"."updated_at", "documents"."secret" FROM "documents" WHERE "documents"."deleted_at" IS NOT NULL`, nil},
		{"pg select only deleted unix", func() (momo.Compiled, error) {
			return momo.Select[sdUnix](pg).OnlyDeleted().SQL()
		}, `SELECT "docs_unix"."id", "docs_unix"."tenant_id", "docs_unix"."title", "docs_unix"."deleted_unix" FROM "docs_unix" WHERE "docs_unix"."deleted_unix" IS NOT NULL`, nil},
		{"pg select default bool", func() (momo.Compiled, error) {
			return momo.Select[sdBool](pg).SQL()
		}, `SELECT "docs_bool"."id", "docs_bool"."tenant_id", "docs_bool"."title", "docs_bool"."deleted" FROM "docs_bool" WHERE "docs_bool"."deleted" = FALSE`, nil},
		{"pg select only deleted bool", func() (momo.Compiled, error) {
			return momo.Select[sdBool](pg).OnlyDeleted().SQL()
		}, `SELECT "docs_bool"."id", "docs_bool"."tenant_id", "docs_bool"."title", "docs_bool"."deleted" FROM "docs_bool" WHERE "docs_bool"."deleted" = TRUE`, nil},
		{"pg select default unix", func() (momo.Compiled, error) {
			return momo.Select[sdUnix](pg).SQL()
		}, `SELECT "docs_unix"."id", "docs_unix"."tenant_id", "docs_unix"."title", "docs_unix"."deleted_unix" FROM "docs_unix" WHERE "docs_unix"."deleted_unix" IS NULL`, nil},
		{"pg select with deleted bool", func() (momo.Compiled, error) {
			return momo.Select[sdBool](pg).WithDeleted().SQL()
		}, `SELECT "docs_bool"."id", "docs_bool"."tenant_id", "docs_bool"."title", "docs_bool"."deleted" FROM "docs_bool"`, nil},
		{"pg select only then with deleted", func() (momo.Compiled, error) {
			return momo.Select[sdBool](pg).OnlyDeleted().WithDeleted().SQL()
		}, `SELECT "docs_bool"."id", "docs_bool"."tenant_id", "docs_bool"."title", "docs_bool"."deleted" FROM "docs_bool"`, nil},
		{"pg soft delete returning", func() (momo.Compiled, error) {
			return momo.SoftDelete[sdBool](pg).Where(sdBoolID.Eq(1)).Returning(sdBoolID).SQL()
		}, `UPDATE "docs_bool" SET "deleted" = $1 WHERE ("docs_bool"."id" = $2) AND ("docs_bool"."deleted" = FALSE) RETURNING "id"`, []any{true, int64(1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.got()
			if err != nil {
				t.Fatal(err)
			}
			if got.SQL != c.sql {
				t.Fatalf("SQL:\n got %s\nwant %s", got.SQL, c.sql)
			}
			if len(got.Args) != len(c.args) {
				t.Fatalf("args = %v, want %v", got.Args, c.args)
			}
			for i := range c.args {
				if got.Args[i] != c.args[i] {
					t.Fatalf("args = %v, want %v", got.Args, c.args)
				}
			}
		})
	}
}

var sdBoolID = momo.NewColumn[int64]("docs_bool", "id", momo.FlagPK)

func TestSoftDeleteGuards(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres), clockAt(sdNow))
	if _, err := momo.SoftDelete[suite.Document](pg).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("unfiltered SoftDelete err = %v, want ErrUnfilteredMutation", err)
	}
	if _, err := momo.SoftDelete[suite.Document](pg).Where(momo.And()).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("Where(And()) SoftDelete err = %v, want ErrUnfilteredMutation", err)
	}
	c, err := momo.SoftDelete[suite.Document](pg).AllRows().SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `UPDATE "documents" SET "deleted_at" = $1 WHERE "documents"."deleted_at" IS NULL`; c.SQL != want {
		t.Fatalf("AllRows SQL:\n got %s\nwant %s", c.SQL, want)
	}
	if _, err := momo.SoftDelete[suite.Document](pg).Where(suite.DocID.Eq(1)).Set(suite.DocTitle.Set("x")).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("SoftDelete with Set err = %v, want ErrValidation", err)
	}
	if _, err := momo.SoftDelete[delNote](pg).Where(delNoteID.Eq(1)).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("SoftDelete on a model without a soft-delete column err = %v, want ErrValidation", err)
	}
	// Plain Delete still refuses; ForceDelete bypasses the refusal but keeps the filter guard.
	if _, err := momo.Delete[suite.Document](pg).Where(suite.DocID.Eq(1)).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("plain Delete err = %v, want ErrValidation", err)
	}
	if _, err := momo.ForceDelete[suite.Document](pg).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("unfiltered ForceDelete err = %v, want ErrUnfilteredMutation", err)
	}
	if _, err := momo.ForceDelete[suite.Document](pg).Where(suite.DocID.Eq(1)).SQL(); err != nil {
		t.Fatalf("ForceDelete err = %v", err)
	}
}
