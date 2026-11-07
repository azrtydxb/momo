// SPDX-License-Identifier: Apache-2.0

// Package momolog provides a log/slog hook for Momo.
package momolog

import (
	"context"
	"log/slog"
	"time"

	"github.com/azrtydxb/momo"
)

type hook struct {
	l      *slog.Logger
	level  slog.Level
	slow   time.Duration
	values bool
}

// Option configures New.
type Option func(*hook)

// Level sets the level for ordinary statements (default Debug).
func Level(l slog.Level) Option { return func(h *hook) { h.level = l } }

// SlowOver logs statements slower than d at Warn.
func SlowOver(d time.Duration) Option { return func(h *hook) { h.slow = d } }

// Values includes bound argument values in the record. It has an effect only
// when the DB was also opened with momo.WithArgLogging; otherwise the event
// carries no values to log.
func Values(on bool) Option { return func(h *hook) { h.values = on } }

// New returns a momo.Hook that logs one record per finished statement with
// the attributes op, table, relation (when set), sql, duration, rows and err
// (when set).
func New(l *slog.Logger, opts ...Option) momo.Hook {
	h := &hook{l: l, level: slog.LevelDebug}
	for _, o := range opts {
		o(h)
	}
	return h
}

func (h *hook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context { return ctx }

func (h *hook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	level := h.level
	if h.slow > 0 && e.Duration > h.slow {
		level = slog.LevelWarn
	}
	if !h.l.Enabled(ctx, level) {
		return
	}
	attrs := make([]slog.Attr, 0, 8)
	attrs = append(attrs, slog.String("op", e.Op))
	if e.Table != "" {
		attrs = append(attrs, slog.String("table", e.Table))
	}
	if e.Relation != "" {
		attrs = append(attrs, slog.String("relation", e.Relation))
	}
	attrs = append(attrs,
		slog.String("sql", e.SQL),
		slog.Duration("duration", e.Duration),
		slog.Int64("rows", e.Rows),
	)
	if h.values && e.Args != nil {
		attrs = append(attrs, slog.Any("args", e.Args))
	}
	if e.Err != nil {
		attrs = append(attrs, slog.String("err", e.Err.Error()))
	}
	h.l.LogAttrs(ctx, level, "momo.query", attrs...)
}
