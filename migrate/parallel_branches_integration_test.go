// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
)

func pbDialect(d dialect.Name) migrate.Dialect {
	switch d {
	case dialect.SQLite:
		return msqlite.New()
	case dialect.MySQL:
		return mmysql.New()
	default:
		return mpg.New()
	}
}

// pbOn writes the dialect into every snapshot and seals the directory with a
// fresh momo.sum, as momo migrate hash would.
func pbOn(t *testing.T, fsys fstest.MapFS, d dialect.Name) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	for k, v := range fsys {
		if k == migrate.SumFileName {
			continue
		}
		data := v.Data
		if strings.HasSuffix(k, ".snapshot.json") {
			data = []byte(strings.ReplaceAll(string(data), `"dialect":"postgres"`, `"dialect":"`+string(d)+`"`))
		}
		out[k] = &fstest.MapFile{Data: data}
	}
	sum, err := migrate.ComputeSum(out)
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := sum.Write(&buf); err != nil {
		t.Fatal(err)
	}
	out[migrate.SumFileName] = pbFile(buf.String())
	return out
}

// pbApplyClean applies the directory and requires the database to match the
// derived schema: the drift report is clean.
func pbApplyClean(t *testing.T, db *momo.DB, d dialect.Name, fsys fstest.MapFS, want int) {
	t.Helper()
	e, err := migrate.NewEngine(db.Driver(), pbOn(t, fsys, d), migrate.WithDialect(pbDialect(d)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rep, err := e.Apply(ctx, migrate.ApplyOptions{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(rep.Applied) != want {
		t.Fatalf("want %d migrations applied, got %+v", want, rep.Applied)
	}
	drift, err := e.DriftWith(ctx, migrate.DriftOptions{})
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	// The test database has extensions installed that no migration manages.
	var unmanaged []migrate.ObjectRef
	for _, o := range drift.Unmanaged {
		if o.Kind != "extension" {
			unmanaged = append(unmanaged, o)
		}
	}
	if len(drift.Changed) != 0 || len(drift.Missing) != 0 || len(unmanaged) != 0 {
		t.Fatalf("the database must match the derived schema after the merge: %+v", drift)
	}
}

// pbSetUp sets the SQL of a migration of the fixture.
func pbSetUp(fsys fstest.MapFS, version, name, sql string) fstest.MapFS {
	fsys[version+"_"+name+".up.sql"] = pbFile("-- momo:generated\n" + sql + "\n")
	return fsys
}

// The merged directory of two branches that added different tables applies,
// and the database then matches the derived schema.
func TestParallelBranchesApply(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		pbApplyClean(t, db, d, pbMergedIndependent(t), 3)
		for _, table := range []string{"base", "a", "b"} {
			mustExecDB(t, db, "INSERT INTO "+table+" (id) VALUES (1)")
		}
	})
}

// A migration built on one branch's migration after the merge applies, and
// the database holds both branches' tables.
func TestParallelBranchesChildOfMergeApply(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		merged := pbMergedIndependent(t)
		b2 := pbBranch(merged, "20260103000000", "add_b2", "CREATE TABLE b2 (id bigint NOT NULL);", pbTable("base"), pbTable("b"), pbTable("b2"))
		b2["20260103000000_add_b2.snapshot.json"] = pbFile(pbSnapshot("20260103000000", pbVB, pbTable("base"), pbTable("b"), pbTable("b2")))
		pbApplyClean(t, db, d, b2, 4)
		for _, table := range []string{"a", "b", "b2"} {
			mustExecDB(t, db, "INSERT INTO "+table+" (id) VALUES (1)")
		}
	})
}

// A fork whose branches change the same table is refused by Apply and nothing runs.
func TestParallelBranchesSameTableApplyRefused(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		base := pbBase()
		a := pbBranch(base, pbVA, "add_x", "ALTER TABLE base ADD COLUMN x text;", pbTable("base", "x"))
		b := pbBranch(base, pbVB, "add_y", "ALTER TABLE base ADD COLUMN y text;", pbTable("base", "y"))
		e, err := migrate.NewEngine(db.Driver(), pbOn(t, pbMerge(t, a, b), d), migrate.WithDialect(pbDialect(d)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Apply(context.Background(), migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrBranchConflict) {
			t.Fatalf("want ErrBranchConflict, got %v", err)
		}
	})
}

// A fork that adds a table with a foreign key to p, and one that adds an
// unrelated table, merge, apply and match.
func TestParallelBranchesForeignKeyForkApply(t *testing.T) {
	db := testopen.Postgres(t)
	root := rootWith(pTable("bigint"))
	pbSetUp(root, pbV0, "init", "CREATE TABLE p (id bigint PRIMARY KEY);")
	a := pbBranchWith(root, pbVA, "add_c", pbTablesOnly(pbVA, pTable("bigint"), childC()))
	pbSetUp(a, pbVA, "add_c", "CREATE TABLE c (id bigint NOT NULL, p_id bigint NOT NULL, CONSTRAINT c_p FOREIGN KEY (p_id) REFERENCES p (id));")
	b := pbBranchWith(root, pbVB, "add_d", pbTablesOnly(pbVB, pTable("bigint"), noKeyTable("d", "bigint")))
	pbSetUp(b, pbVB, "add_d", "CREATE TABLE d (id bigint NOT NULL);")
	pbApplyClean(t, db, dialect.Postgres, pbMerge(t, a, b), 3)
}

// A fork that adds an index to p and one that adds a table merge, apply and match.
func TestParallelBranchesIndexForkApply(t *testing.T) {
	db := testopen.Postgres(t)
	root := rootWith(pTable("bigint"))
	pbSetUp(root, pbV0, "init", "CREATE TABLE p (id bigint PRIMARY KEY);")
	indexed := `{"name":"p","columns":[{"name":"id","type":"bigint"},{"name":"x","type":"text","nullable":true}],"primary_key":["id"],"indexes":[{"name":"p_x_idx","parts":[{"column":"x"}]}]}`
	a := pbBranchWith(root, pbVA, "index_p", pbTablesOnly(pbVA, indexed))
	pbSetUp(a, pbVA, "index_p", "ALTER TABLE p ADD COLUMN x text;\n\nCREATE INDEX p_x_idx ON p (x);")
	b := pbBranchWith(root, pbVB, "add_d", pbTablesOnly(pbVB, pTable("bigint"), noKeyTable("d", "bigint")))
	pbSetUp(b, pbVB, "add_d", "CREATE TABLE d (id bigint NOT NULL);")
	pbApplyClean(t, db, dialect.Postgres, pbMerge(t, a, b), 3)
}

// A fork that adds a view over t and one that changes another table merge,
// apply and match.
func TestParallelBranchesViewForkApply(t *testing.T) {
	db := testopen.Postgres(t)
	root := rootWith(noKeyTable("t", "bigint"), noKeyTable("u", "bigint"))
	pbSetUp(root, pbV0, "init", "CREATE TABLE t (id bigint NOT NULL);\n\nCREATE TABLE u (id bigint NOT NULL);")
	a := pbBranchWith(root, pbVA, "add_v", pbSnapshotWith(pbVA, pbV0, []string{noKeyTable("t", "bigint"), noKeyTable("u", "bigint")}, viewOnT, ""))
	pbSetUp(a, pbVA, "add_v", "CREATE VIEW v AS SELECT id FROM t;")
	widened := `{"name":"u","columns":[{"name":"id","type":"bigint"},{"name":"y","type":"text","nullable":true}]}`
	b := pbBranchWith(root, pbVB, "alter_u", pbTablesOnly(pbVB, noKeyTable("t", "bigint"), widened))
	pbSetUp(b, pbVB, "alter_u", "ALTER TABLE u ADD COLUMN y text;")
	pbApplyClean(t, db, dialect.Postgres, pbMerge(t, a, b), 3)
}
