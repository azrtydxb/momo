// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
)

func goNoop(context.Context, *momo.Tx) error { return nil }

func TestGoMigrationFieldsAreChecked(t *testing.T) {
	good := GoMigration{Version: "20260101000000", Name: "fill", Up: goNoop}
	for name, g := range map[string]GoMigration{
		"short version": {Version: "2026", Name: "fill", Up: goNoop},
		"bad date":      {Version: "20261301000000", Name: "fill", Up: goNoop},
		"bad name":      {Version: "20260101000000", Name: "Fill It", Up: goNoop},
		"no function":   {Version: "20260101000000", Name: "fill"},
		"both":          {Version: "20260101000000", Name: "fill", Up: goNoop, UpNoTx: func(context.Context, *momo.DB) error { return nil }},
	} {
		if err := g.validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if err := good.validate(); err != nil {
		t.Fatal(err)
	}
	if err := checkGoMigrations([]GoMigration{good, good}); err == nil || !strings.Contains(err.Error(), good.Version) {
		t.Fatalf("a repeated Go version must be an error naming it, got %v", err)
	}
}

func TestGoMigrationIdentityIsStable(t *testing.T) {
	a := GoMigration{Version: "20260101000000", Name: "fill", Checksum: "v1", Up: goNoop}
	if a.identity() != a.identity() || len(a.identity()) != 64 {
		t.Fatalf("identity must be a stable sha256 hex, got %q", a.identity())
	}
	b, c, d := a, a, a
	b.Checksum = "v2"
	c.Name = "other"
	d.Version = "20260102000000"
	for _, x := range []GoMigration{b, c, d} {
		if x.identity() == a.identity() {
			t.Fatalf("identity must change with checksum, name and version: %+v", x)
		}
	}
	// The function is not part of the identity: Go code cannot be hashed.
	e := a
	e.Up = func(context.Context, *momo.Tx) error { return context.Canceled }
	if e.identity() != a.identity() {
		t.Fatal("identity must not depend on the function")
	}
}

func goDir() fstest.MapFS {
	return fstest.MapFS{
		"20260101000000_root.up.sql":        {Data: []byte("-- momo:generated\nCREATE TABLE a (id bigint);")},
		"20260101000000_root.snapshot.json": {Data: []byte(snapTable("20260101000000", "", "a"))},
		"20260102000000_fill.snapshot.json": {Data: []byte(snapTable("20260102000000", "20260101000000", "a"))},
	}
}

func TestValidateOfflineChecksGoMigrationSnapshot(t *testing.T) {
	g := GoMigration{Version: "20260102000000", Name: "fill", Up: goNoop}
	rep, err := ValidateOffline(sealed(t, goDir()), g)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 0 {
		t.Fatalf("a Go migration whose snapshot equals its parent's is clean, got %v", rep.Problems)
	}

	// A snapshot that changes the schema is refused, like a custom SQL migration's.
	changed := goDir()
	changed["20260102000000_fill.snapshot.json"] = &fstest.MapFile{Data: []byte(snapTable("20260102000000", "20260101000000", "b"))}
	rep, err = ValidateOffline(sealed(t, changed), g)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 1 || !strings.Contains(rep.Problems[0], "must not change the schema") || !strings.Contains(rep.Problems[0], "20260102000000_fill") {
		t.Fatalf("want one schema-change problem, got %v", rep.Problems)
	}

	// A Go migration without a snapshot file is reported.
	missing := goDir()
	delete(missing, "20260102000000_fill.snapshot.json")
	rep, err = ValidateOffline(sealed(t, missing), g)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 1 || !strings.Contains(rep.Problems[0], "has no snapshot file") {
		t.Fatalf("want one missing-snapshot problem, got %v", rep.Problems)
	}

	// A Go migration on the version of a SQL file is reported.
	clash := GoMigration{Version: "20260101000000", Name: "clash", Up: goNoop}
	rep, err = ValidateOffline(sealed(t, goDir()), clash)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range rep.Problems {
		found = found || strings.Contains(p, "same version as Go migration")
	}
	if !found {
		t.Fatalf("want a version-collision problem, got %v", rep.Problems)
	}
}

func TestGoCollisionsNamesTheFile(t *testing.T) {
	files, err := Discover(goDir())
	if err != nil {
		t.Fatal(err)
	}
	g := GoMigration{Version: "20260101000000", Name: "clash", Up: goNoop}
	if err := goCollisions([]GoMigration{g}, files); err == nil || !strings.Contains(err.Error(), "20260101000000_root.up.sql") {
		t.Fatalf("want an error naming the SQL file, got %v", err)
	}
	g.Version = "20260102000000" // only a snapshot file has this version
	if err := goCollisions([]GoMigration{g}, files); err != nil {
		t.Fatalf("a snapshot file is the Go migration's own, got %v", err)
	}
}
