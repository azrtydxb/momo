// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"strings"
	"testing"
)

func TestDirectives(t *testing.T) {
	body := "-- momo:no-transaction\n-- momo:destructive\nDROP TABLE x;"
	d, err := readDirectives([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !d.NoTransaction || !d.Destructive || d.Split {
		t.Fatalf("bad directives: %+v", d)
	}
	d2, err := readDirectives([]byte("DROP TABLE x;\n-- momo:destructive"))
	if err != nil {
		t.Fatal(err)
	}
	if d2.Destructive {
		t.Fatal("a directive below the first statement must not count")
	}
}

func TestDirectivesImported(t *testing.T) {
	d, err := readDirectives([]byte("-- momo:imported\n-- momo:no-transaction\nCREATE TABLE x (id int);"))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Imported || !d.NoTransaction || d.Generated {
		t.Fatalf("bad directives: %+v", d)
	}
}

func TestDirectivesHostile(t *testing.T) {
	_, err := readDirectives([]byte("-- a note\n-- momo:destructiv\nDROP TABLE x;"))
	if err == nil || !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "destructiv") {
		t.Fatalf("want positioned unknown-directive error, got %v", err)
	}
	d, err := readDirectives([]byte("\xef\xbb\xbf-- momo:split\r\n-- momo:generated\r\nSELECT 1;"))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Split || !d.Generated {
		t.Fatalf("BOM/CRLF directives lost: %+v", d)
	}
	d, err = readDirectives([]byte("-- momo:sqlite-foreign-keys-off\n"))
	if err != nil || !d.SQLiteForeignKeysOff {
		t.Fatalf("sqlite-foreign-keys-off: %+v %v", d, err)
	}
	if _, err := readDirectives(nil); err != nil {
		t.Fatalf("empty body: %v", err)
	}
}
