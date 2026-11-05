// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo/schema"
)

func TestSnapshotParentRecorded(t *testing.T) {
	var buf bytes.Buffer
	in := SnapshotFile{Version: "20260102000000", ParentVersion: "20260101000000", Schema: &schema.Schema{Dialect: "postgres"}}
	if err := WriteSnapshot(&buf, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadSnapshot(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if out.ParentVersion != "20260101000000" || out.Version != "20260102000000" {
		t.Fatalf("parent or version lost: %+v", out)
	}
}

func TestSnapshotFormatVersion(t *testing.T) {
	src := `{"format_version":9,"version":"1","parent":"","schema":{"dialect":"postgres","tables":[]}}`
	_, err := ReadSnapshot(strings.NewReader(src))
	if !errors.Is(err, schema.ErrNewerFormat) {
		t.Fatalf("want ErrNewerFormat, got %v", err)
	}
}

func TestSnapshotNewerSchemaFormat(t *testing.T) {
	src := `{"format_version":1,"version":"1","parent":"","schema":{"format_version":9,"dialect":"postgres","tables":[]}}`
	if _, err := ReadSnapshot(strings.NewReader(src)); !errors.Is(err, schema.ErrNewerFormat) {
		t.Fatalf("want ErrNewerFormat, got %v", err)
	}
}

func snapJSON(version, parent string) string {
	return `{"format_version":1,"version":"` + version + `","parent":"` + parent + `","schema":{"format_version":1,"dialect":"postgres","tables":[]}}`
}

func TestSnapshotCorrupt(t *testing.T) {
	good := snapJSON("20260101000000", "")
	for name, src := range map[string]string{
		"empty":        "",
		"truncated":    good[:len(good)/2],
		"garbage":      "not json",
		"no format":    `{"version":"1","schema":{"format_version":1}}`,
		"no schema":    `{"format_version":1,"version":"1"}`,
		"null schema":  `{"format_version":1,"version":"1","schema":null}`,
		"schema no fv": `{"format_version":1,"version":"1","schema":{"dialect":"postgres"}}`,
		"trailing":     good + `{}`,
	} {
		if _, err := ReadSnapshot(strings.NewReader(src)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestSnapshotBOMAndCRLF(t *testing.T) {
	src := "\xEF\xBB\xBF" + strings.ReplaceAll(snapJSON("20260101000000", ""), ",", ",\r\n")
	out, err := ReadSnapshot(strings.NewReader(src))
	if err != nil || out.Version != "20260101000000" {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestSnapshotWriteDeterministic(t *testing.T) {
	mk := func(order []string) SnapshotFile {
		s := &schema.Schema{Dialect: "postgres"}
		for _, n := range order {
			s.Tables = append(s.Tables, schema.Table{Name: n})
		}
		return SnapshotFile{Version: "1", Schema: s}
	}
	var a, b bytes.Buffer
	in := mk([]string{"b", "a"})
	if err := WriteSnapshot(&a, in); err != nil {
		t.Fatal(err)
	}
	if err := WriteSnapshot(&b, mk([]string{"a", "b"})); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatalf("not byte-identical:\n%s\n%s", a.String(), b.String())
	}
	if in.Schema.Tables[0].Name != "b" {
		t.Fatal("WriteSnapshot mutated its input")
	}
	if !bytes.HasSuffix(a.Bytes(), []byte("\n")) {
		t.Fatal("missing trailing newline")
	}
}

func TestWriteSnapshotRejectsNilSchema(t *testing.T) {
	if err := WriteSnapshot(&bytes.Buffer{}, SnapshotFile{Version: "1"}); err == nil {
		t.Fatal("want error")
	}
}

func TestLatestSnapshot(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		s, err := LatestSnapshot(fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("x")}})
		if s != nil || !errors.Is(err, ErrNoSnapshot) {
			t.Fatalf("got %v, %v", s, err)
		}
	})
	t.Run("greatest", func(t *testing.T) {
		s, err := LatestSnapshot(fstest.MapFS{
			"20260102000000_b.snapshot.json": {Data: []byte(snapJSON("20260102000000", "20260101000000"))},
			"20260101000000_a.snapshot.json": {Data: []byte(snapJSON("20260101000000", ""))},
		})
		if err != nil || s.Version != "20260102000000" {
			t.Fatalf("got %+v, %v", s, err)
		}
	})
	t.Run("duplicate version", func(t *testing.T) {
		_, err := LatestSnapshot(fstest.MapFS{
			"20260101000000_a.snapshot.json": {Data: []byte(snapJSON("20260101000000", ""))},
			"20260101000000_b.snapshot.json": {Data: []byte(snapJSON("20260101000000", ""))},
		})
		if err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("version mismatch with file name", func(t *testing.T) {
		_, err := LatestSnapshot(fstest.MapFS{
			"20260101000000_a.snapshot.json": {Data: []byte(snapJSON("20260109000000", ""))},
		})
		if err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("self parent", func(t *testing.T) {
		_, err := LatestSnapshot(fstest.MapFS{
			"20260101000000_a.snapshot.json": {Data: []byte(snapJSON("20260101000000", "20260101000000"))},
		})
		if err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("newer format", func(t *testing.T) {
		_, err := LatestSnapshot(fstest.MapFS{
			"20260101000000_a.snapshot.json": {Data: []byte(`{"format_version":9,"version":"20260101000000","schema":{}}`)},
		})
		if !errors.Is(err, ErrNewerFormat) {
			t.Fatalf("got %v", err)
		}
	})
}
