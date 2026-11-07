// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momolog_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/momolog"
)

// capture runs one parameterized select for a value that matches nothing, on
// each dialect, and passes the log text of that select to verify.
func capture(t *testing.T, hookOpts []momolog.Option, dbOpts []momo.Option, verify func(t *testing.T, out string)) {
	t.Helper()
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	opts := append([]momo.Option{momo.WithHook(momolog.New(l, hookOpts...))}, dbOpts...)
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, db)
		buf.Reset()
		if _, err := momo.Select[models.Author](db).Where(author.Name.Eq("s3cr3t-needle")).All(context.Background()); err != nil {
			t.Fatal(err)
		}
		verify(t, buf.String())
	}, opts...)
}

func TestSlogHook(t *testing.T) {
	t.Run("record has op table sql duration and rows, no values", func(t *testing.T) {
		capture(t, []momolog.Option{momolog.Level(slog.LevelDebug)}, nil, func(t *testing.T, out string) {
			for _, want := range []string{"level=DEBUG", "op=select", "table=authors", "sql=", "FROM", "duration=", "rows=0"} {
				if !strings.Contains(out, want) {
					t.Fatalf("log lacks %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "s3cr3t-needle") {
				t.Fatalf("argument value leaked:\n%s", out)
			}
		})
	})

	t.Run("WithArgLogging alone does not log values", func(t *testing.T) {
		capture(t, nil, []momo.Option{momo.WithArgLogging()}, func(t *testing.T, out string) {
			if strings.Contains(out, "s3cr3t-needle") {
				t.Fatalf("argument value leaked:\n%s", out)
			}
		})
	})

	t.Run("Values alone is not enough, it also needs WithArgLogging", func(t *testing.T) {
		capture(t, []momolog.Option{momolog.Values(true)}, nil, func(t *testing.T, out string) {
			if strings.Contains(out, "s3cr3t-needle") {
				t.Fatalf("value logged without WithArgLogging:\n%s", out)
			}
		})
	})

	t.Run("Values plus WithArgLogging logs the value", func(t *testing.T) {
		capture(t, []momolog.Option{momolog.Values(true)}, []momo.Option{momo.WithArgLogging()}, func(t *testing.T, out string) {
			if !strings.Contains(out, "s3cr3t-needle") {
				t.Fatalf("value missing with Values(true) and WithArgLogging:\n%s", out)
			}
		})
	})

	t.Run("slow statements log at WARN, fast ones at DEBUG", func(t *testing.T) {
		capture(t, []momolog.Option{momolog.SlowOver(time.Nanosecond)}, nil, func(t *testing.T, out string) {
			if !strings.Contains(out, "level=WARN") {
				t.Fatalf("slow statement not at WARN:\n%s", out)
			}
		})
		capture(t, []momolog.Option{momolog.SlowOver(time.Hour)}, nil, func(t *testing.T, out string) {
			if !strings.Contains(out, "level=DEBUG") {
				t.Fatalf("fast statement not at DEBUG:\n%s", out)
			}
		})
	})

	t.Run("errors are logged", func(t *testing.T) {
		var buf bytes.Buffer
		l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
			buf.Reset()
			if _, err := momo.Exec(context.Background(), db, `SELECT * FROM no_such_table`); err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(buf.String(), "err=") {
				t.Fatalf("log lacks err=:\n%s", buf.String())
			}
		}, momo.WithHook(momolog.New(l)))
	})
}
