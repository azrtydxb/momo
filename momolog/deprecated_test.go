// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests exist to exercise the deprecated names

package momolog_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/momolog"
)

// Catches: a deprecated option name that stops forwarding to its With*
// replacement, so a caller that has not migrated yet logs differently.
func TestDeprecatedOptionsMatchReplacements(t *testing.T) {
	event := &momo.QueryEvent{Op: "select", SQL: "select 1", Duration: 2 * time.Second, Args: []any{"x"}}
	run := func(opts ...momolog.Option) string {
		var buf bytes.Buffer
		// Drop the timestamp so two runs of one event render the same text.
		l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			},
		}))
		h := momolog.New(l, opts...)
		h.AfterQuery(context.Background(), event)
		return buf.String()
	}
	cases := []struct {
		name     string
		old, now []momolog.Option
	}{
		{"Level", []momolog.Option{momolog.Level(slog.LevelInfo)}, []momolog.Option{momolog.WithLevel(slog.LevelInfo)}},
		{"SlowOver", []momolog.Option{momolog.SlowOver(time.Second)}, []momolog.Option{momolog.WithSlowThreshold(time.Second)}},
		{"Values", []momolog.Option{momolog.Values(true)}, []momolog.Option{momolog.WithValues(true)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, want := run(c.old...), run(c.now...); got != want {
				t.Errorf("deprecated option logs\n%s\nwant\n%s", got, want)
			}
		})
	}
}
