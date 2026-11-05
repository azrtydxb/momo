// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver_test

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// allowedDiffs is the capability matrix for the database/sql adapter. A
// capability may differ from the native pgx adapter only when it is listed
// here with its reason.
var allowedDiffs = map[string]string{
	"Copy":  "momo-03 Task 9 gives FromSQLDB no COPY path; CopyFrom is Task 8, on the native pool and transactions only",
	"Batch": "momo-03 Task 9 gives FromSQLDB no batch path; SendBatch is Task 8, on the native pool and transactions only",
}

func TestPostgresSQLDBAdapter(t *testing.T) {
	sqldb, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	viaSQL := momo.New(pgxdriver.FromSQLDB(sqldb))
	native := testopen.Postgres(t)

	suite.RunAll(t, viaSQL, suite.CapsOf(viaSQL))

	got := reflect.ValueOf(suite.CapsOf(viaSQL))
	want := reflect.ValueOf(suite.CapsOf(native))
	differs := map[string]bool{}
	for i := 0; i < got.NumField(); i++ {
		f := got.Field(i)
		name := got.Type().Field(i).Name
		if f.Kind() != reflect.Bool || f.Bool() == want.Field(i).Bool() {
			continue
		}
		differs[name] = true
		if _, ok := allowedDiffs[name]; !ok {
			t.Errorf("capability %s differs: database/sql=%v native=%v; list it in allowedDiffs with a reason",
				name, f.Bool(), want.Field(i).Bool())
		}
	}
	for name := range allowedDiffs {
		if !differs[name] {
			t.Errorf("allowedDiffs lists %s, but it no longer differs; remove the entry", name)
		}
	}
}
