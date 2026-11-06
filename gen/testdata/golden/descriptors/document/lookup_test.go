// SPDX-License-Identifier: Apache-2.0

package document

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

func TestColumnByNameRuntime(t *testing.T) {
	if _, err := ColumnByName("unknown"); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("unknown name: %v", err)
	}
	c, err := ColumnByName("title")
	if err != nil || c == nil {
		t.Fatalf("title: %v %v", c, err)
	}
}

func allowed() map[string]bool {
	m := map[string]bool{}
	for _, n := range ColumnNames() {
		m[n] = true
	}
	return m
}

func TestColumnByNameHostile(t *testing.T) {
	hostile := []string{
		"", " ", "Title", "TITLE", "title ", " title", "title\n", "title\x00",
		`"title"`, "`title`", "'title'", "documents.title", "title.", ".title",
		"title; DROP TABLE documents", "title--", "title/*x*/", "title OR 1=1",
		"title,id", "title\tid", "id desc", "1", "*", "title​", "tıtle",
	}
	for _, h := range hostile {
		c, err := ColumnByName(h)
		if c != nil || !errors.Is(err, momo.ErrValidation) {
			t.Errorf("ColumnByName(%q) = %v, %v; want rejection", h, c, err)
		}
	}
	// every allowlisted name resolves to a column of that exact name.
	for _, n := range ColumnNames() {
		c, err := ColumnByName(n)
		if err != nil || c == nil || c.ColumnName() != n {
			t.Errorf("ColumnByName(%q) = %v, %v", n, c, err)
		}
	}
}

func FuzzColumnByName(f *testing.F) {
	for _, s := range []string{"", "title", "Title", "id", `"id"`, "a b", "x;y", "documents.id", "\x00"} {
		f.Add(s)
	}
	ok := allowed()
	f.Fuzz(func(t *testing.T, name string) {
		c, err := ColumnByName(name)
		if ok[name] {
			if err != nil || c == nil || c.ColumnName() != name {
				t.Fatalf("allowlisted %q rejected: %v", name, err)
			}
			return
		}
		if c != nil || err == nil {
			t.Fatalf("non-allowlisted %q accepted", name)
		}
	})
}
