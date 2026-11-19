// SPDX-License-Identifier: Apache-2.0

package sqlitedriver

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
)

// Catches: New accepting an option it cannot apply, which would leave the
// caller believing a busy timeout or a connection limit is set on a pool that
// was opened without it.
func TestNewRefusesOptions(t *testing.T) {
	d := New(nil, WithBusyTimeout(time.Second), WithMaxOpenConns(4))
	_, err := d.Exec(t.Context(), "SELECT 1", nil)
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Exec err = %v, want ErrValidation", err)
	}
	for _, want := range []string{"WithBusyTimeout", "WithMaxOpenConns"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
	if strings.Contains(err.Error(), "WithTxLock") {
		t.Errorf("error %q names an option that was not given", err)
	}
	if _, err := d.Query(t.Context(), "SELECT 1", nil); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Query err = %v, want ErrValidation", err)
	}
	if _, err := d.Begin(t.Context(), momo.TxOptions{}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Begin err = %v, want ErrValidation", err)
	}
	// An option that sets nothing is not a setting.
	if err := refusedOptions(config([]Option{WithConfig(Config{})})); err != nil {
		t.Fatalf("WithConfig of the zero Config refused: %v", err)
	}
}
