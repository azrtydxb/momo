// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// roDoc has an identity key and a read-only column.
type roDoc struct {
	ID   int64
	Name string
	Calc string
}

var roDocMeta = &momo.Meta[roDoc]{
	Table: "ro_docs",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
		{Name: "name"},
		{Name: "calc", Flags: momo.FlagReadOnly},
	},
	Ptr: func(m *roDoc, i int) any {
		return [...]any{&m.ID, &m.Name, &m.Calc}[i]
	},
	Value: func(m *roDoc, i int) any {
		return [...]any{m.ID, m.Name, m.Calc}[i]
	},
	Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*roDoc) MomoMeta() *momo.Meta[roDoc] { return roDocMeta }

// A Default on an identity or read-only column is ignored: the column never
// reaches the SQL, for Values and for Model with Omit.
func TestFieldDefaultOnNonWritableColumnIsIgnored(t *testing.T) {
	name := momo.NewColumn[string]("ro_docs", "name", 0)
	id := momo.NewColumn[int64]("ro_docs", "id", momo.FlagPK|momo.FlagIdentity)
	calc := momo.NewColumn[string]("ro_docs", "calc", momo.FlagReadOnly)
	for _, tc := range []struct {
		name string
		col  int
		ref  string
	}{{"identity", 0, `"id"`}, {"read-only", 2, `"calc"`}} {
		t.Run(tc.name, func(t *testing.T) {
			db := momo.New(fakedriver.New(dialect.Postgres), momo.WithFieldHooks[roDoc](
				[]momo.FieldHook[roDoc]{{Column: tc.col, Default: func(*roDoc) any { return int64(99) }}}))
			c, err := momo.Insert[roDoc](db).Values(name.Set("a")).SQL()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(c.SQL, tc.ref) || len(c.Args) != 1 {
				t.Fatalf("Values wrote the non-writable column: %q %v", c.SQL, c.Args)
			}
			c, err = momo.Insert[roDoc](db).Model(&roDoc{Name: "a"}).Omit(id, calc).SQL()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(c.SQL, tc.ref) || len(c.Args) != 1 {
				t.Fatalf("Model+Omit wrote the non-writable column: %q %v", c.SQL, c.Args)
			}
		})
	}
}

// The same rule on the suite's Document, whose id is an identity column.
func TestFieldDefaultOnIdentityDocument(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithFieldHooks[suite.Document](
		[]momo.FieldHook[suite.Document]{{Column: 0, Default: func(*suite.Document) any { return int64(5) }}}))
	c, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("a")).SQL()
	if err != nil || strings.Contains(c.SQL, `"id"`) {
		t.Fatalf("%q %v", c.SQL, err)
	}
}
