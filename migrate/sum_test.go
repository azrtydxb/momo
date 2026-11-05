// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSumFormatVersion(t *testing.T) {
	fsys := fstest.MapFS{"20260101120000_a.up.sql": {Data: []byte("SELECT 1;")}}
	s, err := ComputeSum(fsys)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "# momo.sum format=1\n") {
		t.Fatalf("missing header: %q", buf.String())
	}
	future := strings.Replace(buf.String(), "format=1", "format=9", 1)
	if _, err := ParseSum(strings.NewReader(future)); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("want ErrNewerFormat, got %v", err)
	}
}

func TestChecksumMismatch(t *testing.T) {
	orig := fstest.MapFS{"20260101120000_a.up.sql": {Data: []byte("SELECT 1;")}}
	s, err := ComputeSum(orig)
	if err != nil {
		t.Fatal(err)
	}
	edited := fstest.MapFS{"20260101120000_a.up.sql": {Data: []byte("SELECT 2;")}}
	bad := VerifySum(edited, s)
	if len(bad) != 1 || bad[0] != "20260101120000_a.up.sql" {
		t.Fatalf("edited file not reported: %v", bad)
	}
}

func sumFS() fstest.MapFS {
	return fstest.MapFS{
		"20260102000000_b.up.sql": {Data: []byte("SELECT 2;")},
		"20260101000000_a.up.sql": {Data: []byte("SELECT 1;")},
		"20260103000000_c.up.sql": {Data: []byte("SELECT 3;")},
		"momo.sum":                {Data: []byte("ignored")},
	}
}

func TestSumDeterministicOrderAndRoundTrip(t *testing.T) {
	var first string
	for i := 0; i < 20; i++ {
		s, err := ComputeSum(sumFS())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := s.Write(&buf); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = buf.String()
			continue
		}
		if buf.String() != first {
			t.Fatalf("output differs between runs:\n%s\n%s", first, buf.String())
		}
	}
	lines := strings.Split(strings.TrimSuffix(first, "\n"), "\n")
	if len(lines) != 5 || !strings.HasPrefix(lines[1], "h1:") {
		t.Fatalf("unexpected layout: %q", first)
	}
	for i, want := range []string{"20260101000000_a.up.sql", "20260102000000_b.up.sql", "20260103000000_c.up.sql"} {
		if !strings.HasSuffix(lines[2+i], "  "+want) {
			t.Fatalf("line %d = %q, want name %s", 2+i, lines[2+i], want)
		}
	}
	got, err := ParseSum(strings.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := ComputeSum(sumFS())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: got %+v want %+v", got, want)
	}
	if bad := VerifySum(sumFS(), got); len(bad) != 0 {
		t.Fatalf("clean directory reported %v", bad)
	}
}

func TestVerifySumReportsAddedAndRemoved(t *testing.T) {
	s, _ := ComputeSum(sumFS())
	fsys := sumFS()
	delete(fsys, "20260101000000_a.up.sql")
	fsys["20260104000000_d.up.sql"] = &fstest.MapFile{Data: []byte("SELECT 4;")}
	fsys["20260102000000_b.up.sql"] = &fstest.MapFile{Data: []byte("tampered")}
	got := VerifySum(fsys, s)
	want := []string{"20260101000000_a.up.sql", "20260102000000_b.up.sql", "20260104000000_d.up.sql"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseSumRejectsTampering(t *testing.T) {
	s, _ := ComputeSum(sumFS())
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	good := buf.String()
	lines := strings.Split(good, "\n")
	flipped := strings.Replace(good, lines[2][:4], "ffff", 1)
	cases := map[string]string{
		"no header":      strings.Join(lines[1:], "\n"),
		"format zero":    strings.Replace(good, "format=1", "format=0", 1),
		"bad hash line":  strings.Replace(good, "h1:", "h2:", 1),
		"entry edited":   flipped,
		"dirhash edited": strings.Replace(good, lines[1], "h1:AAAA", 1),
		"short hex":      strings.Replace(good, lines[2], "abcd  20260101000000_a.up.sql", 1),
		"one space":      strings.Replace(good, "  20260101000000_a.up.sql", " 20260101000000_a.up.sql", 1),
		"duplicate":      good + lines[2] + "\n",
		"empty":          "",
	}
	for name, src := range cases {
		if _, err := ParseSum(strings.NewReader(src)); err == nil {
			t.Errorf("%s: tampered file accepted", name)
		}
	}
	for name, src := range map[string]string{
		"entry edited":   flipped,
		"dirhash edited": strings.Replace(good, lines[1], "h1:AAAA", 1),
	} {
		if _, err := ParseSum(strings.NewReader(src)); !errors.Is(err, ErrChecksumMismatch) {
			t.Errorf("%s: want ErrChecksumMismatch, got %v", name, err)
		}
	}
	// CRLF and a BOM, as written by an editor on Windows, still parse.
	crlf := "\xef\xbb\xbf" + strings.ReplaceAll(good, "\n", "\r\n")
	if _, err := ParseSum(strings.NewReader(crlf)); err != nil {
		t.Fatalf("CRLF/BOM: %v", err)
	}
}

func TestComputeSumRejectsUnwritableNames(t *testing.T) {
	fsys := fstest.MapFS{"bad\nname.sql": {Data: []byte("x")}}
	if _, err := ComputeSum(fsys); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("want error naming the file, got %v", err)
	}
}

func TestAppendSum(t *testing.T) {
	base := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("SELECT 1;")}}
	existing, err := ComputeSum(base)
	if err != nil {
		t.Fatal(err)
	}
	// An already-recorded file is edited on disk: AppendSum must keep its old line.
	next := fstest.MapFS{
		"20260101000000_a.up.sql": {Data: []byte("EDITED")},
		"20260102000000_b.up.sql": {Data: []byte("SELECT 2;")},
	}
	got, err := AppendSum(existing, next, []string{"20260102000000_b.up.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Files["20260101000000_a.up.sql"] != existing.Files["20260101000000_a.up.sql"] {
		t.Fatal("existing entry was changed")
	}
	if len(got.Files) != 2 || got.DirHash == existing.DirHash {
		t.Fatalf("new entry or dirhash missing: %+v", got)
	}
	if bad := VerifySum(next, got); !reflect.DeepEqual(bad, []string{"20260101000000_a.up.sql"}) {
		t.Fatalf("edit of an old file must still be detected, got %v", bad)
	}
	if len(existing.Files) != 1 {
		t.Fatal("AppendSum mutated its input")
	}
	if _, err := AppendSum(existing, next, []string{"20260101000000_a.up.sql"}); err == nil || !strings.Contains(err.Error(), "20260101000000_a.up.sql") {
		t.Fatalf("re-adding a recorded file must fail naming it, got %v", err)
	}
	if _, err := AppendSum(existing, next, []string{"20260109000000_nope.up.sql"}); err == nil {
		t.Fatal("adding a missing file must fail")
	}
}

func TestSumWalksNestedDirectories(t *testing.T) {
	fsys := fstest.MapFS{
		"20260301000000_c.up.sql":              {Data: []byte("C")},
		"archive/2026/20260101000000_a.up.sql": {Data: []byte("A")},
		"archive/momo.sum":                     {Data: []byte("not the root sum")},
		"momo.sum":                             {Data: []byte("root")},
	}
	s, err := ComputeSum(fsys)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"20260301000000_c.up.sql", "archive/2026/20260101000000_a.up.sql", "archive/momo.sum"} {
		if _, ok := s.Files[name]; !ok {
			t.Fatalf("%s must be recorded, got %v", name, s.Files)
		}
	}
	if _, ok := s.Files["momo.sum"]; ok {
		t.Fatal("the root momo.sum must not be recorded")
	}
	if bad := VerifySum(fsys, s); bad != nil {
		t.Fatalf("clean tree must verify, got %v", bad)
	}
	fsys["archive/2026/20260101000000_a.up.sql"] = &fstest.MapFile{Data: []byte("edited")}
	if bad := VerifySum(fsys, s); len(bad) != 1 || bad[0] != "archive/2026/20260101000000_a.up.sql" {
		t.Fatalf("edit of a nested file must be reported by its slash path, got %v", bad)
	}
	delete(fsys, "archive/2026/20260101000000_a.up.sql")
	if bad := VerifySum(fsys, s); len(bad) != 1 {
		t.Fatalf("removal of a nested file must be reported, got %v", bad)
	}
	added, err := AppendSum(&Sum{DirHash: dirHash(map[string]string{}), Files: map[string]string{}}, fsys, []string{"archive/momo.sum"})
	if err != nil || len(added.Files) != 1 {
		t.Fatalf("AppendSum of a nested path: %v %v", added, err)
	}
}
