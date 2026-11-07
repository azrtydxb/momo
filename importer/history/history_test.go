// SPDX-License-Identifier: Apache-2.0

package history

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo/migrate"
)

func TestConvertKeepsTimestampsAndMapsOrdinals(t *testing.T) {
	src := Source{Tool: "goose", Migrations: []Raw{
		{Version: "00001", Name: "init", Up: []byte("CREATE TABLE a (x int);"), Down: []byte("DROP TABLE a;"), Origin: "00001_init.sql"},
		{Version: "20260101000000", Name: "more", Up: []byte("CREATE TABLE b (x int);"), Origin: "20260101000000_more.sql"},
	}}
	conv, rep, err := Convert(src)
	if err != nil {
		t.Fatal(err)
	}
	if conv[0].Version != "19700101000001" || conv[0].OriginalVersion != "00001" || conv[0].Down == nil {
		t.Fatalf("mapped version wrong: %+v", conv[0])
	}
	if conv[1].Version != "20260101000000" {
		t.Fatalf("timestamp version must be kept: %+v", conv[1])
	}
	if !rep.Has("version-mapped") {
		t.Fatalf("mapping not recorded in the report: %s", rep)
	}
	again, _, err := Convert(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(again[0].Up) != string(conv[0].Up) || again[0].Version != conv[0].Version {
		t.Fatal("conversion must be deterministic")
	}
	rows := HistoryRows(conv, nil, "goose", time.Unix(0, 0))
	if rows[0].Name != "00001_init" {
		t.Fatalf("original version must be kept in the name column: %q", rows[0].Name)
	}
}

func TestDirtySourceRefused(t *testing.T) {
	_, _, err := Convert(Source{Tool: "golangmigrate", Dirty: true})
	if !errors.Is(err, ErrSourceDirty) {
		t.Fatalf("dirty source must be refused, got %v", err)
	}
}

func TestConvertRefusals(t *testing.T) {
	_, _, err := Convert(Source{Tool: "atlas", Unsupported: []string{"x.hcl"}})
	if !errors.Is(err, ErrUnsupportedSource) || !strings.Contains(err.Error(), "x.hcl") {
		t.Fatalf("unsupported files must be refused by name, got %v", err)
	}
	_, _, err = Convert(Source{Tool: "goose", Applied: []string{"1"}, Migrations: []Raw{
		{Version: "1", Name: "a", Origin: "1_a.sql"},
		{Version: "2", Name: "b", Origin: "2_b.sql"},
	}})
	if !errors.Is(err, ErrSourcePending) {
		t.Fatalf("unapplied source migrations must be refused, got %v", err)
	}
	// A timestamp migration followed by a mapped one would sort backwards.
	_, _, err = Convert(Source{Tool: "goose", Migrations: []Raw{
		{Version: "20260101000000", Name: "a", Origin: "a.sql"},
		{Version: "2", Name: "b", Origin: "b.sql"},
	}})
	if !errors.Is(err, ErrUnsupportedSource) {
		t.Fatalf("out-of-order mapping must be refused, got %v", err)
	}
}

func TestConvertMappedVersionsStayValidTimestamps(t *testing.T) {
	var raws []Raw
	for i := 1; i <= 130; i++ {
		raws = append(raws, Raw{Version: strings.Repeat("0", 5) + string(rune('a'+i%26)), Name: "m", Origin: "f"})
	}
	conv, _, err := Convert(Source{Tool: "bun", Migrations: raws})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range conv {
		if _, err := time.Parse(versionLayout, c.Version); err != nil {
			t.Fatalf("mapped version %s is not a timestamp: %v", c.Version, err)
		}
	}
}

func TestFileNameSanitised(t *testing.T) {
	for in, want := range map[string]string{
		"Add Users":   "add_users",
		"20_x-y":      "m_20_x_y",
		"":            "m",
		"create_user": "create_user",
	} {
		if got := fileName(in); got != want {
			t.Errorf("fileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteImportNeverOverwrites(t *testing.T) {
	conv := []Converted{{Version: "20260101000000", OriginalVersion: "20260101000000", Name: "init", Up: []byte("CREATE TABLE a (x int);\n"), Down: []byte("DROP TABLE a;\n"), NoTx: true}}
	dir := filepath.Join(t.TempDir(), "migrations")
	if err := WriteImport(dir, conv, nil); err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile(filepath.Join(dir, "20260101000000_init.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(up), "-- momo:no-transaction\n") {
		t.Fatalf("a NoTx source must carry the directive: %q", up)
	}
	for _, f := range []string{"20260101000000_init.down.sql", migrate.SumFileName} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteImport(dir, conv, nil); err == nil {
		t.Fatal("WriteImport overwrote an existing file")
	}
	sumFile, err := os.Open(filepath.Join(dir, migrate.SumFileName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sumFile.Close() }()
	sum, err := migrate.ParseSum(sumFile)
	if err != nil {
		t.Fatal(err)
	}
	rows := HistoryRows(conv, sum, "goose", time.Unix(0, 0))
	if rows[0].Checksum == "" || rows[0].Checksum != sum.Files["20260101000000_init.up.sql"] {
		t.Fatalf("history checksum must come from momo.sum: %+v", rows[0])
	}
	if rows[0].Status != migrate.StatusApplied || rows[0].ApplyID != "import:goose" || rows[0].Kind != "sql" {
		t.Fatalf("history row wrong: %+v", rows[0])
	}
}
