// SPDX-License-Identifier: Apache-2.0

package atlas

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

const fixtureDir = "../../../internal/compat/atlasfixture/migrations"

func fixture(t *testing.T) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	for _, n := range []string{"20260101000000_init.sql", "20260101000001_more.sql", SumFile} {
		b, err := fs.ReadFile(os.DirFS(fixtureDir), n)
		if err != nil {
			t.Fatal(err)
		}
		m[n] = &fstest.MapFile{Data: b}
	}
	return m
}

func TestReadVerifiesSumAndRevisions(t *testing.T) {
	src, err := Read(fixture(t), []Revision{{Version: "20260101000000", Applied: 1, Total: 1}})
	if err != nil {
		t.Fatalf("the committed fixture must verify: %v", err)
	}
	if len(src.Migrations) != 2 || src.Migrations[0].Version != "20260101000000" || src.Migrations[0].Down != nil {
		t.Fatalf("migrations misread: %+v", src.Migrations)
	}
	if len(src.Applied) != 1 || src.Dirty {
		t.Fatalf("one applied revision misread: applied=%v dirty=%v", src.Applied, src.Dirty)
	}
	src, err = Read(fixture(t), []Revision{{Version: "20260101000000", Applied: 1, Total: 2, Error: "boom"}})
	if err != nil {
		t.Fatal(err)
	}
	if !src.Dirty || len(src.Applied) != 0 {
		t.Fatalf("a failed revision must be dirty and not applied: %+v", src)
	}
}

func TestReadRefusesEditedFile(t *testing.T) {
	fsys := fixture(t)
	fsys["20260101000001_more.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE b (x bigint);\n")}
	_, err := Read(fsys, nil)
	if err == nil || !strings.Contains(err.Error(), "20260101000001_more.sql") {
		t.Fatalf("an edited file must be refused by name, got %v", err)
	}
}

func TestReadRefusesMissingSum(t *testing.T) {
	fsys := fixture(t)
	delete(fsys, SumFile)
	if _, err := Read(fsys, nil); err == nil {
		t.Fatal("a directory without atlas.sum must be refused")
	}
}

func TestReadReadsTxmodeNone(t *testing.T) {
	body := "-- atlas:txmode none\n\nCREATE INDEX CONCURRENTLY i ON a (x);\n"
	names := []string{"20260101000002_idx.sql"}
	bodies := map[string][]byte{names[0]: []byte(body)}
	total, files := computeSum(names, bodies)
	sum := "h1:" + total + "\n" + files[0].name + " h1:" + files[0].hash + "\n"
	fsys := fstest.MapFS{
		names[0]: {Data: []byte(body)},
		SumFile:  {Data: []byte(sum)},
	}
	src, err := Read(fsys, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !src.Migrations[0].NoTx {
		t.Fatal("atlas:txmode none must set NoTx")
	}
}

func TestReadRefusesAppliedRevisionWithoutFile(t *testing.T) {
	_, err := Read(fixture(t), []Revision{
		{Version: "20260101000000", Applied: 1, Total: 1},
		{Version: "20250101000000", Applied: 1, Total: 1},
	})
	if err == nil || !strings.Contains(err.Error(), "20250101000000") {
		t.Fatalf("an applied revision without a file must be refused by version, got %v", err)
	}
}
