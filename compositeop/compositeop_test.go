// SPDX-License-Identifier: Apache-2.0

package compositeop_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/compositeop"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/types"
)

type addr struct {
	Street string
	Floor  int32
}

var colHome = momo.NewColumn[types.Composite[addr]]("docs", "home", 0)

// Catches: an operand bound without the cast to the composite type (the server
// then reads the parameter as an anonymous record and refuses it), a type name
// that is not quoted, a schema that is dropped from a qualified name, and a
// value interpolated instead of bound.
func TestCompositePredicatesGolden(t *testing.T) {
	v := types.Composite[addr]{V: addr{Street: "Main", Floor: 3}}
	for _, c := range []struct {
		name string
		pred momo.Predicate
		want string
	}{
		{"equal", compositeop.Equals(colHome, "address", v), `"docs"."home" = $1::"address"`},
		{"not equal", compositeop.NotEquals(colHome, "address", v), `"docs"."home" <> $1::"address"`},
		{"qualified", compositeop.Equals(colHome, "billing.address", v), `"docs"."home" = $1::"billing"."address"`},
	} {
		got, err := exprtest.Compile(dialect.Postgres, c.pred)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if want := `SELECT COUNT(*) FROM "docs" WHERE ` + c.want; got.SQL != want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{v}) {
			t.Errorf("%s: args = %#v, want the bound value", c.name, got.Args)
		}
	}
}

// Catches: a composite comparison silently rendered on a dialect with no
// composite type.
func TestCompositePredicatesUnsupportedOffPostgres(t *testing.T) {
	v := types.Composite[addr]{}
	preds := map[string]momo.Predicate{
		"equal":     compositeop.Equals(colHome, "address", v),
		"not equal": compositeop.NotEquals(colHome, "address", v),
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		for name, p := range preds {
			if _, err := exprtest.Compile(d, p); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s %s: err = %v, want ErrUnsupported", d, name, err)
			}
		}
	}
}
