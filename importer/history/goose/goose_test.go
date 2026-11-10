// SPDX-License-Identifier: Apache-2.0

package goose

import (
	"strings"
	"testing"
)

func TestSplitGoose(t *testing.T) {
	up, down, err := splitGoose([]byte("-- +goose Up\nCREATE TABLE a (x int);\n\n-- +goose Down\nDROP TABLE a;\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(up) != "CREATE TABLE a (x int);" || string(down) != "DROP TABLE a;" {
		t.Fatalf("up=%q down=%q", up, down)
	}
}

func TestSplitGooseKeepsStatementBlockWhole(t *testing.T) {
	body := "-- +goose Up\n-- +goose StatementBegin\nCREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql;\n-- +goose StatementEnd\n"
	up, down, noTx, err := parseFile([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if down != nil || noTx {
		t.Fatalf("down=%q noTx=%v", down, noTx)
	}
	if !strings.HasPrefix(string(up), "CREATE FUNCTION f()") || !strings.HasSuffix(string(up), "plpgsql;") || strings.Contains(string(up), "+goose") {
		t.Fatalf("the block must stay whole without the annotations: %q", up)
	}
}

func TestParseNoTransaction(t *testing.T) {
	_, _, noTx, err := parseFile([]byte("-- +goose Up\n-- +goose NO TRANSACTION\nCREATE INDEX CONCURRENTLY i ON a (x);\n"))
	if err != nil || !noTx {
		t.Fatalf("noTx=%v err=%v", noTx, err)
	}
}

func TestParseRefusesBadFiles(t *testing.T) {
	for name, body := range map[string]string{
		"no up section":      "CREATE TABLE a (x int);\n",
		"unknown annotation": "-- +goose Up\n-- +goose Frobnicate\nSELECT 1;\n",
		"unclosed block":     "-- +goose Up\n-- +goose StatementBegin\nSELECT 1;\n",
	} {
		if _, _, err := splitGoose([]byte(body)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
