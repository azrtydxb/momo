// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestValidateChannel(t *testing.T) {
	for _, ok := range []string{"orders", "Orders", "a b", `we"ird`, strings.Repeat("x", 63)} {
		if err := validateChannel(ok); err != nil {
			t.Errorf("validateChannel(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("x", 64), "a\x00b"} {
		if err := validateChannel(bad); err == nil {
			t.Errorf("validateChannel(%q) = nil, want error", bad)
		}
	}
}

func TestQuoteChannel(t *testing.T) {
	cases := map[string]string{
		"orders":   `"orders"`,
		"Orders":   `"Orders"`,
		`we"ird`:   `"we""ird"`,
		"x; DROP":  `"x; DROP"`,
		"with spc": `"with spc"`,
	}
	for in, want := range cases {
		if got := quoteChannel(in); got != want {
			t.Errorf("quoteChannel(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestListenRejectsBadArguments(t *testing.T) {
	cfg := &pgx.ConnConfig{}
	ctx := context.Background()
	if _, err := Listen(ctx, nil, []string{"c"}); err == nil {
		t.Error("nil config: want error")
	}
	if _, err := Listen(ctx, cfg, nil); err == nil {
		t.Error("no channels: want error")
	}
	if _, err := Listen(ctx, cfg, []string{""}); err == nil {
		t.Error("empty channel: want error")
	}
	for _, b := range [][2]time.Duration{{0, time.Second}, {2 * time.Second, time.Second}} {
		if _, err := Listen(ctx, cfg, []string{"c"}, WithBackoff(b[0], b[1])); err == nil {
			t.Errorf("WithBackoff(%v, %v): want error", b[0], b[1])
		}
	}
}

func TestNextDelayDoublesAndCaps(t *testing.T) {
	c := listenConfig{minBackoff: 10 * time.Millisecond, maxBackoff: 35 * time.Millisecond}
	var d time.Duration
	var got []time.Duration
	for range 4 {
		d = nextDelay(d, c)
		got = append(got, d)
	}
	want := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 35 * time.Millisecond, 35 * time.Millisecond}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("delays = %v, want %v", got, want)
		}
	}
}
