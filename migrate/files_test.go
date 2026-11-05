// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestDiscoverRejectsMalformedNames(t *testing.T) {
	fsys := fstest.MapFS{
		"20260101120000_init.up.sql":        {Data: []byte("CREATE TABLE a (x int);")},
		"20260101120000_init.snapshot.json": {Data: []byte("{}")},
		"20260101120000_bad name.up.sql":    {Data: []byte("")},
	}
	if _, err := Discover(fsys); err == nil || !strings.Contains(err.Error(), "bad name") {
		t.Fatalf("want error naming the malformed file, got %v", err)
	}
}

func TestDiscoverHostile(t *testing.T) {
	bad := map[string]fstest.MapFS{
		"uppercase name":    {"20260101120000_Init.up.sql": {}},
		"missing kind":      {"20260101120000_init.sql": {}},
		"short version":     {"2026010112000_init.up.sql": {}},
		"impossible date":   {"20261340000000_init.up.sql": {}},
		"duplicate version": {"20260101120000_a.up.sql": {}, "20260101120000_b.up.sql": {}},
		"two names":         {"20260101120000_a.up.sql": {}, "20260101120000_b.down.sql": {}},
	}
	for name, fsys := range bad {
		_, err := Discover(fsys)
		if err == nil || !strings.Contains(err.Error(), "migrate: ") || !strings.Contains(err.Error(), "2026") {
			t.Errorf("%s: want error naming the file, got %v", name, err)
		}
	}
}

func TestDiscoverSortedAndIgnores(t *testing.T) {
	fsys := fstest.MapFS{
		"20260102000000_b.up.sql":        {Data: []byte("B")},
		"20260101000000_a.up.sql":        {Data: []byte("A")},
		"20260101000000_a.down.sql":      {Data: []byte("D")},
		"20260101000000_a.snapshot.json": {Data: []byte("{}")},
		"momo.sum":                       {Data: []byte("x")},
		"README.md":                      {Data: []byte("x")},
		"sub/notes.txt":                  {Data: []byte("x")},
	}
	got, err := Discover(fsys)
	if err != nil {
		t.Fatal(err)
	}
	var paths, kinds []string
	for _, f := range got {
		paths = append(paths, f.Path)
		kinds = append(kinds, f.Kind)
	}
	wantP := "20260101000000_a.down.sql 20260101000000_a.snapshot.json 20260101000000_a.up.sql 20260102000000_b.up.sql"
	if strings.Join(paths, " ") != wantP {
		t.Fatalf("paths = %v", paths)
	}
	if strings.Join(kinds, " ") != "down snapshot up up" {
		t.Fatalf("kinds = %v", kinds)
	}
	if got[3].Version != "20260102000000" || got[3].Name != "b" || string(got[3].Body) != "B" {
		t.Fatalf("bad file: %+v", got[3])
	}
}

func TestDiscoverWalksNestedDirectoriesOrderedByVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"20260301000000_c.up.sql":                     {Data: []byte("C")},
		"archive/2026/20260101000000_a.up.sql":        {Data: []byte("A")},
		"archive/2026/20260101000000_a.snapshot.json": {Data: []byte("{}")},
		"archive/20260201000000_b.up.sql":             {Data: []byte("B")},
		"momo.sum":                                    {Data: []byte("x")},
		"archive/momo.sum":                            {Data: []byte("x")},
		"archive/README.md":                           {Data: []byte("x")},
	}
	got, err := Discover(fsys)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range got {
		paths = append(paths, f.Path)
	}
	want := "archive/2026/20260101000000_a.snapshot.json archive/2026/20260101000000_a.up.sql archive/20260201000000_b.up.sql 20260301000000_c.up.sql"
	if strings.Join(paths, " ") != want {
		t.Fatalf("paths = %v\nwant   %s", paths, want)
	}
	if string(got[1].Body) != "A" {
		t.Fatalf("nested body = %q", got[1].Body)
	}
}

func TestDiscoverRejectsVersionInTwoDirectories(t *testing.T) {
	fsys := fstest.MapFS{
		"20260101000000_a.up.sql":         {Data: []byte("A")},
		"archive/20260101000000_a.up.sql": {Data: []byte("A")},
	}
	_, err := Discover(fsys)
	if err == nil || !strings.Contains(err.Error(), "archive/20260101000000_a.up.sql") || !strings.Contains(err.Error(), " 20260101000000_a.up.sql") {
		t.Fatalf("want error naming both paths, got %v", err)
	}
	split := fstest.MapFS{
		"20260101000000_a.up.sql":                {Data: []byte("A")},
		"archive/20260101000000_a.snapshot.json": {Data: []byte("{}")},
	}
	if _, err := Discover(split); err == nil || !strings.Contains(err.Error(), "archive/20260101000000_a.snapshot.json") {
		t.Fatalf("one version split across directories must be refused, got %v", err)
	}
}
