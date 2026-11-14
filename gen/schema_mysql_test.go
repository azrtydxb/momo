// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func mysqlCfg() Config { return Config{Dialects: []dialect.Name{dialect.MySQL}} }

func diagText(diags []Diagnostic) string {
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Msg)
	}
	return strings.Join(msgs, "\n")
}

// Catches: a string without size:N in a primary key, unique key or index
// reaching MySQL as text, where the server refuses it with error 1170 in the
// middle of a migration and the non-transactional DDL leaves it dirty. The
// schema must be refused before anything is written, naming the column and
// size:N; a sized string, and the same text column outside any key, stay fine.
func TestMySQLRefusesUnsizedTextInKeys(t *testing.T) {
	bad := []struct {
		name string
		f    *Field
	}{
		{"primary key", fld("Code", "string", "pk")},
		{"unique", fld("Code", "string", "unique")},
		{"index", fld("Code", "string", "index")},
		{"blob index", fld("Code", "[]byte", "index")},
		{"json unique", fld("Code", "types.JSON[map[string]string]", "unique")},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			fields := []*Field{fld("ID", "int64", "pk,identity"), c.f}
			if c.name == "primary key" {
				fields = []*Field{c.f}
			}
			_, diags := build(t, mysqlCfg(), mdl("Thing", "things", fields...))
			got := diagText(diags)
			if !strings.Contains(got, "column code") || !strings.Contains(got, "size:N") {
				t.Fatalf("diagnostics %q must name the column and size:N", got)
			}
		})
	}
	ok := []*Field{fld("Code", "string", "unique,size:40"), fld("Code", "string", ""), fld("Code", "[]byte", "")}
	for _, f := range ok {
		if _, diags := build(t, mysqlCfg(), mdl("Thing", "things", fld("ID", "int64", "pk,identity"), f)); len(diags) != 0 {
			t.Errorf("%s: unexpected diagnostics %v", f.GoType, diags)
		}
	}
	// PostgreSQL keys text freely.
	if _, diags := build(t, pg(), mdl("Thing", "things", fld("Code", "string", "pk"))); len(diags) != 0 {
		t.Errorf("PostgreSQL refused a text key: %v", diags)
	}
}

// Catches: a method:fulltext index on a text column being refused as an unkeyable
// text index, although MySQL renders it as FULLTEXT and takes the column whole;
// and, in the other direction, the exemption widening to a btree index, which
// still fails with error 1170 on MySQL.
func TestMySQLFulltextIndexTakesUnsizedText(t *testing.T) {
	for _, method := range []string{"fulltext", "FULLTEXT", "spatial"} {
		f := fld("Body", "string", "index,method:"+method)
		if _, diags := build(t, mysqlCfg(), mdl("Doc", "docs", fld("ID", "int64", "pk,identity"), f)); len(diags) != 0 {
			t.Errorf("method:%s on unsized text: unexpected diagnostics %v", method, diags)
		}
	}
	for _, method := range []string{"btree", ""} {
		tag := "index"
		if method != "" {
			tag += ",method:" + method
		}
		_, diags := build(t, mysqlCfg(), mdl("Doc", "docs", fld("ID", "int64", "pk,identity"), fld("Body", "string", tag)))
		if got := diagText(diags); !strings.Contains(got, "column body") || !strings.Contains(got, "size:N") {
			t.Errorf("method %q index on unsized text: diagnostics %q must still refuse it", method, got)
		}
	}
}

// Catches: a DECIMAL that MySQL cannot create (precision above 65, scale above
// 30 or above the precision) passing the generator, and a valid one losing its
// type; precision without scale is scale 0.
func TestMySQLDecimalLimits(t *testing.T) {
	bad := map[string]string{
		"precision:66":          "above 65",
		"precision:20,scale:31": "above 30",
		"precision:5,scale:6":   "above precision",
	}
	for tag, want := range bad {
		_, diags := build(t, mysqlCfg(), mdl("Thing", "things", fld("ID", "int64", "pk,identity"), fld("Price", "types.Numeric", tag)))
		if !strings.Contains(diagText(diags), want) {
			t.Errorf("%s: diagnostics %q lack %q", tag, diagText(diags), want)
		}
	}
	for tag, want := range map[string]string{"precision:65,scale:30": "decimal(65,30)", "precision:12,scale:2": "decimal(12,2)", "precision:12": "decimal(12)"} {
		s, diags := build(t, mysqlCfg(), mdl("Thing", "things", fld("ID", "int64", "pk,identity"), fld("Price", "types.Numeric", tag)))
		if len(diags) != 0 {
			t.Errorf("%s: diagnostics %v", tag, diags)
			continue
		}
		if got := mustCol(t, s, "things", "price").Type; got != want {
			t.Errorf("%s: type %q, want %q", tag, got, want)
		}
	}
	// The limits are MySQL's: PostgreSQL takes a larger numeric.
	if _, diags := build(t, pg(), mdl("Thing", "things", fld("Price", "types.Numeric", "precision:70"))); len(diags) != 0 {
		t.Errorf("PostgreSQL refused precision 70: %v", diags)
	}
}
