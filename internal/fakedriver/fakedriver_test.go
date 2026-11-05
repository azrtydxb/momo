// SPDX-License-Identifier: Apache-2.0

package fakedriver

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestFakeDriverRecordsAndScripts(t *testing.T) {
	d := New(dialect.SQLite)
	d.SetRows([][]any{{int64(1), "a"}, {int64(2), "b"}})
	rows, err := d.Query(context.Background(), "SELECT 1", []any{"x"})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	var name string
	n := 0
	for rows.Next() {
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		n++
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if n != 2 || id != 2 || name != "b" {
		t.Fatalf("scripted rows wrong: n=%d id=%d name=%s", n, id, name)
	}
	if d.Closed() != 1 || len(d.Calls()) != 1 || d.Calls()[0].SQL != "SELECT 1" {
		t.Fatalf("recording wrong: closed=%d calls=%v", d.Closed(), d.Calls())
	}
}
