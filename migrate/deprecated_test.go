// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	iredact "github.com/azrtydxb/momo/internal/redact"
	"github.com/azrtydxb/momo/internal/sqlerr"
	"github.com/azrtydxb/momo/internal/sqlsplit"
)

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Catches: a deprecated forwarder drifting from the function it stands for.
func TestDeprecatedForwarders(t *testing.T) {
	for _, m := range []string{"", "plain", "postgres://u:secret@h/db?sslmode=disable", "password=hunter2 host=h"} {
		if got, want := RedactURLs(m, "hunter2"), iredact.RedactURLs(m, "hunter2"); got != want {
			t.Errorf("RedactURLs(%q) = %q, want %q", m, got, want)
		}
	}
	for _, body := range []string{"", "SELECT 1; SELECT 2;", "-- momo:split\nSELECT 1; SELECT 2;"} {
		for _, d := range []string{"postgres", "sqlite", "mysql"} {
			g1, e1 := SplitStatements([]byte(body), d)
			g2, e2 := sqlsplit.Split([]byte(body), d)
			if !reflect.DeepEqual(g1, g2) || errText(e1) != errText(e2) {
				t.Errorf("SplitStatements(%q, %s) = %v, %v; want %v, %v", body, d, g1, e1, g2, e2)
			}
		}
	}
	no := errors.New("no such table: momo_migrations")
	for _, err := range []error{nil, no, errors.New("other")} {
		if got, want := IsNoSuchTable(err, "momo_migrations"), sqlerr.IsNoSuchTable(err, "momo_migrations"); got != want {
			t.Errorf("IsNoSuchTable(%v) = %v, want %v", err, got, want)
		}
	}
	for _, body := range []string{"", "-- momo:no-transaction\nSELECT 1;", "-- momo:bogus\nSELECT 1;"} {
		d1, e1 := ParseDirectives([]byte(body))
		d2, e2 := readDirectives([]byte(body))
		if d1 != d2 || errText(e1) != errText(e2) {
			t.Errorf("ParseDirectives(%q) differs from readDirectives", body)
		}
	}
	fsys := fstest.MapFS{"20260101000000_init.up.sql": {Data: []byte("SELECT 1;")}}
	f1, e1 := Discover(fsys)
	f2, e2 := discover(fsys)
	if !reflect.DeepEqual(f1, f2) || errText(e1) != errText(e2) || len(f1) != 1 {
		t.Errorf("Discover = %v, %v; want %v, %v", f1, e1, f2, e2)
	}
	now := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	v1, e1 := NextVersion(now, fsys)
	v2, e2 := nextVersion(now, fsys)
	if v1 != v2 || errText(e1) != errText(e2) || v1 != "20260101000001" {
		t.Errorf("NextVersion = %q, %v; want %q, %v", v1, e1, v2, e2)
	}
	c1, e1 := Chain(fstest.MapFS{})
	c2, e2 := snapshotChain(fstest.MapFS{})
	if !reflect.DeepEqual(c1, c2) || errText(e1) != errText(e2) {
		t.Errorf("Chain = %v, %v; want %v, %v", c1, e1, c2, e2)
	}
	for _, rows := range [][]HistoryRow{nil, {{Version: "1", Kind: "sql"}}, {{Version: "2", Name: "g", Kind: "go"}}} {
		g1, g2 := RefuseGoRollback(rows), refuseGoRollback(rows)
		if errText(g1) != errText(g2) || errors.Is(g1, ErrGoMigrationNotReversible) != errors.Is(g2, ErrGoMigrationNotReversible) {
			t.Errorf("RefuseGoRollback(%v) = %v, want %v", rows, g1, g2)
		}
	}
}

// Catches: the directive list of the statement splitter and the directive
// parser of migrate falling out of step: a name one accepts and the other
// treats as unknown ends the header scan early or fails a valid file.
func TestDirectiveListsAgree(t *testing.T) {
	for _, name := range sqlsplit.KnownDirectives {
		if _, err := parseDirectives([]byte("-- momo:" + name + "\nSELECT 1;")); err != nil {
			t.Errorf("parseDirectives rejects the known directive %q: %v", name, err)
		}
	}
	if _, err := parseDirectives([]byte("-- momo:bogus\nSELECT 1;")); err == nil {
		t.Error("parseDirectives accepts an unknown directive")
	}
	// A split directive behind every other known one is still found.
	for _, name := range sqlsplit.KnownDirectives {
		if name == "split" {
			continue
		}
		body := "-- momo:" + name + "\n-- momo:split\nSELECT 1; SELECT 2;"
		got, err := sqlsplit.Split([]byte(body), "postgres")
		if err != nil || len(got) != 2 {
			t.Errorf("Split after %q = %v, %v; want two statements", name, got, err)
		}
	}
}
