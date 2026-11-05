// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"testing"
	"testing/fstest"
)

// TestWalkSkipsDotEntries pins the walk rule: dotfiles and dot directories
// (editor, VCS, OS and momo temp files) are not content of the directory.
func TestWalkSkipsDotEntries(t *testing.T) {
	sql := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	fsys := fstest.MapFS{
		"20260101000000_a.up.sql":                sql("CREATE TABLE a (id int);\n"),
		"archive/20250101000000_old.up.sql":      sql("CREATE TABLE o (id int);\n"),
		".DS_Store":                              sql("junk"),
		".momo-123.tmp":                          sql("partial"),
		"archive/.momo-9.tmp":                    sql("partial"),
		".git/HEAD":                              sql("ref"),
		".hidden/20240101000000_x.up.sql":        sql("CREATE TABLE x (id int);\n"),
		"archive/.cache/20240101000001_y.up.sql": sql("CREATE TABLE y (id int);\n"),
	}
	sum, err := ComputeSum(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Files) != 2 {
		t.Fatalf("ComputeSum must hash only the two real files, got %v", sum.Files)
	}
	files, err := Discover(fsys)
	if err != nil || len(files) != 2 {
		t.Fatalf("Discover must see only the two real files: %v %v", files, err)
	}
	if bad := VerifySum(fsys, sum); bad != nil {
		t.Fatalf("VerifySum flagged %v", bad)
	}
}
