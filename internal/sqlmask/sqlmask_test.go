// SPDX-License-Identifier: Apache-2.0

package sqlmask

import (
	"errors"
	"testing"
)

func TestConversionErrHidesQuotedValue(t *testing.T) {
	never := func(error) bool { return false }
	in := errors.New(`sql: Scan error on column index 0, name "x": converting driver.Value type string ("secret-value") to a int: invalid syntax`)
	got := ConversionErr(in, "fixed", never)
	if got.Error() != "fixed" || !errors.Is(got, in) {
		t.Fatalf("got %q; want the fixed text with the cause reachable", got)
	}
	other := errors.New("boom")
	if ConversionErr(other, "fixed", never) != other {
		t.Error("an unrelated error was masked")
	}
	if ConversionErr(in, "fixed", func(error) bool { return true }) != in {
		t.Error("a driver error was masked")
	}
	if ConversionErr(nil, "fixed", never) != nil {
		t.Error("nil became an error")
	}
}
