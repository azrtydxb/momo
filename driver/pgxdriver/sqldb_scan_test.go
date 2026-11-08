// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/internal/sqlstub"
)

// database/sql quotes the stored value in a conversion failure; the adapter's
// Scan must not pass that text on.
func TestSQLRowsScanMasksConversionError(t *testing.T) {
	db := sqlstub.Open("secret-value")
	defer db.Close()
	r, err := db.Query("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	rows := sqlRows{r}
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

func TestSQLRowsScanStillWorks(t *testing.T) {
	db := sqlstub.Open("hello")
	defer db.Close()
	r, err := db.Query("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	rows := sqlRows{r}
	defer rows.Close()
	var s string
	if !rows.Next() || rows.Scan(&s) != nil || s != "hello" {
		t.Fatalf("Scan = %q", s)
	}
}
