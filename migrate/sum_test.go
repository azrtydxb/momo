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
	if !strings.HasPrefix(buf.String(), "# momo.sum format=2\n") {
		t.Fatalf("missing header: %q", buf.String())
	}
	if strings.Contains(buf.String(), "h1:") {
		t.Fatalf("format 2 has no directory hash: %q", buf.String())
	}
	future := strings.Replace(buf.String(), "format=2", "format=9", 1)
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
	if len(lines) != 4 || lines[0] != "# momo.sum format=2" {
		t.Fatalf("unexpected layout: %q", first)
	}
	for i, want := range []string{"20260101000000_a.up.sql", "20260102000000_b.up.sql", "20260103000000_c.up.sql"} {
		if !strings.HasSuffix(lines[1+i], "  "+want) {
			t.Fatalf("line %d = %q, want name %s", 1+i, lines[1+i], want)
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
	s.Format = SumFormatLegacy
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
	if len(got.Files) != 2 || got.Format != SumFormat || got.DirHash != "" {
		t.Fatalf("new entry missing or format changed: %+v", got)
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

const (
	hashA = "1111111111111111111111111111111111111111111111111111111111111111"
	hashB = "2222222222222222222222222222222222222222222222222222222222222222"
)

// Format 2 lines may come in any order, as after a union merge of two branches.
func TestParseSumFormat2AnyOrder(t *testing.T) {
	src := "# momo.sum format=2\n" + hashB + "  20260102000000_b.up.sql\n" + hashA + "  20260101000000_a.up.sql\n"
	s, err := ParseSum(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if s.Format != 2 || s.DirHash != "" || len(s.Files) != 2 || s.Files["20260101000000_a.up.sql"] != hashA {
		t.Fatalf("parsed %+v", s)
	}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	want := "# momo.sum format=2\n" + hashA + "  20260101000000_a.up.sql\n" + hashB + "  20260102000000_b.up.sql\n"
	if buf.String() != want {
		t.Fatalf("Write must sort the lines:\n%s", buf.String())
	}
}

// A union merge can repeat a line; a name with two checksums, an h1: line and
// conflict markers are refused.
func TestParseSumFormat2Refusals(t *testing.T) {
	head := "# momo.sum format=2\n"
	line := hashA + "  20260101000000_a.up.sql\n"
	if _, err := ParseSum(strings.NewReader(head + line + line)); err != nil {
		t.Fatalf("an identical repeated line is a harmless union-merge artefact: %v", err)
	}
	for name, src := range map[string]string{
		"two checksums":    head + line + hashB + "  20260101000000_a.up.sql\n",
		"h1 line":          head + "h1:AAAA\n" + line,
		"conflict marker":  head + "<<<<<<< HEAD\n" + line + "=======\n>>>>>>> branch\n",
		"format 1 header":  "# momo.sum format=1\n" + line,
		"future format":    "# momo.sum format=3\n" + line,
		"missing the name": head + hashA + "\n",
	} {
		_, err := ParseSum(strings.NewReader(src))
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
		if name == "future format" && !errors.Is(err, ErrNewerFormat) {
			t.Errorf("a format above this momo's must be ErrNewerFormat, got %v", err)
		}
	}
}

// Format 1 still parses and verifies, is written on request and survives
// AppendSum; only hash upgrades it.
func TestSumFormat1StaysFormat1(t *testing.T) {
	base := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("SELECT 1;")}}
	s, err := ComputeSum(base)
	if err != nil {
		t.Fatal(err)
	}
	s.Format = SumFormatLegacy
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "# momo.sum format=1\nh1:") {
		t.Fatalf("format 1 layout: %q", buf.String())
	}
	old, err := ParseSum(strings.NewReader(buf.String()))
	if err != nil || old.Format != SumFormatLegacy || old.DirHash == "" {
		t.Fatalf("format 1 must parse: %+v %v", old, err)
	}
	next := fstest.MapFS{
		"20260101000000_a.up.sql": base["20260101000000_a.up.sql"],
		"20260102000000_b.up.sql": {Data: []byte("SELECT 2;")},
	}
	got, err := AppendSum(old, next, []string{"20260102000000_b.up.sql"})
	if err != nil || got.Format != SumFormatLegacy || got.DirHash == old.DirHash {
		t.Fatalf("AppendSum must keep format 1 and refresh its hash: %+v %v", got, err)
	}
	up, err := ComputeSum(next)
	if err != nil || up.Format != SumFormat {
		t.Fatalf("ComputeSum writes format 2: %+v %v", up, err)
	}
	if err := (&Sum{Format: 7, Files: map[string]string{}}).Write(&buf); err == nil {
		t.Fatal("an unknown format must not be written")
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
