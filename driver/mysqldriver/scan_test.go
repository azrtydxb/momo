// SPDX-License-Identifier: Apache-2.0

package mysqldriver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/sqlstub"
)

// database/sql quotes the stored value in a conversion failure; Scan must not
// pass that text on.
func TestScanMasksConversionError(t *testing.T) {
	db := sqlstub.Open("secret-value")
	defer db.Close()
	rows, err := New(db, dialect.FlavorMySQL).Query(context.Background(), "SELECT 1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("no row: %v", rows.Err())
	}
	var n int
	err = rows.Scan(&n)
	if err == nil {
		t.Fatal("Scan of a string into an int succeeded")
	}
	if strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("the error quotes the stored value: %v", err)
	}
	if errors.Unwrap(err) == nil {
		t.Fatal("the cause is not reachable")
	}
}

func TestScanStillWorks(t *testing.T) {
	db := sqlstub.Open("hello")
	defer db.Close()
	rows, err := New(db, dialect.FlavorMySQL).Query(context.Background(), "SELECT 1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var s string
	if !rows.Next() || rows.Scan(&s) != nil || s != "hello" {
		t.Fatalf("Scan = %q", s)
	}
}

func TestConfigureDSNRefusesWeakeningSettings(t *testing.T) {
	for _, dsn := range []string{
		"u:pw@tcp(127.0.0.1:3306)/db?allowAllFiles=true",
		"u:pw@tcp(127.0.0.1:3306)/db?allowCleartextPasswords=true",
	} {
		_, err := ConfigureDSN(dsn)
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("ConfigureDSN(%q) err = %v, want ErrValidation", dsn, err)
		}
		if err != nil && strings.Contains(err.Error(), "pw") {
			t.Errorf("the error quotes the DSN: %v", err)
		}
	}
	for _, dsn := range []string{
		"u:pw@tcp(127.0.0.1:3306)/db?allowAllFiles=false&allowCleartextPasswords=false",
		"u:pw@tcp(127.0.0.1:3306)/db",
	} {
		if _, err := ConfigureDSN(dsn); err != nil {
			t.Errorf("ConfigureDSN(%q) refused: %v", dsn, err)
		}
	}
}

// An unknown flavor must fail Open, not hand back a driver whose every call
// fails. Open returns before it dials, so no server is needed.
func TestOpenUnknownFlavor(t *testing.T) {
	db, drv, err := Open("u:pw@tcp(127.0.0.1:1)/db", dialect.Flavor("bogus"))
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Open err = %v, want ErrUnsupported", err)
	}
	if db != nil || drv != nil {
		t.Fatalf("Open returned db=%v driver=%v with an error", db, drv)
	}
}

// Open pings, so an unreachable server fails here. Port 1 refuses at once.
func TestOpenPingsServer(t *testing.T) {
	db, drv, err := Open("u:pw@tcp(127.0.0.1:1)/db", dialect.FlavorMySQL)
	if err == nil {
		_ = db.Close()
		t.Fatal("Open succeeded without a server")
	}
	if db != nil || drv != nil {
		t.Fatalf("Open returned db=%v driver=%v with an error", db, drv)
	}
}
