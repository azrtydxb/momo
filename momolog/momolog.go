// SPDX-License-Identifier: Apache-2.0

// Package momolog provides a log/slog hook for Momo.
package momolog

import (
	"context"
	"log/slog"
	"time"

	"github.com/azrtydxb/momo"
)

// Hook is the slog hook New returns. It implements momo.Hook and
// momo.RelationHook, so one momo.WithHook call registers it. The zero value is
// not usable; create it with New.
type Hook struct {
	l      *slog.Logger
	level  slog.Level
	slow   time.Duration
	values bool
}

// Option configures New.
type Option func(*Hook)

// Level sets the level for ordinary statements (default Debug).
func Level(l slog.Level) Option { return func(h *Hook) { h.level = l } }

// SlowOver logs statements slower than d at Warn.
func SlowOver(d time.Duration) Option { return func(h *Hook) { h.slow = d } }

// Values includes bound argument values in the record. It has an effect only
// when the DB was also opened with momo.WithArgLogging; otherwise the event
// carries no values to log.
func Values(on bool) Option { return func(h *Hook) { h.values = on } }

// New returns a hook that logs one "momo.query" record per finished statement
// with the attributes op, table, relation (when set), sql, duration, rows and
// err (when set). The hook also implements momo.RelationHook, so momo.WithHook
// additionally logs one "momo.relation" record per finished relation load with
// parent, name, kind, parents, children, queries, duration and err (when set).
// Errors are logged through Error() only. Argument values are logged only with
// Values(true) and momo.WithArgLogging together, and a value bound from a
// sensitive column is momo.RedactedArg in them.
func New(l *slog.Logger, opts ...Option) *Hook {
	h := &Hook{l: l, level: slog.LevelDebug}
	for _, o := range opts {
		o(h)
	}
	return h
}

// BeforeQuery does nothing.
func (h *Hook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context { return ctx }

// AfterQuery logs the finished statement.
func (h *Hook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
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

var (
	_ momo.Hook         = (*Hook)(nil)
	_ momo.RelationHook = (*Hook)(nil)
)

// BeforeRelation does nothing.
func (h *Hook) BeforeRelation(ctx context.Context, _ *momo.RelationEvent) context.Context {
	return ctx
}

// AfterRelation logs the finished relation load.
func (h *Hook) AfterRelation(ctx context.Context, e *momo.RelationEvent) {
	if !h.l.Enabled(ctx, h.level) {
		return
	}
	attrs := []slog.Attr{
		slog.String("parent", e.Parent),
		slog.String("name", e.Name),
		slog.String("kind", e.Kind.String()),
		slog.Int("parents", e.Parents),
		slog.Int64("children", e.Children),
		slog.Int("queries", e.Queries),
		slog.Duration("duration", e.Duration),
	}
	if e.Err != nil {
		attrs = append(attrs, slog.String("err", e.Err.Error()))
	}
	h.l.LogAttrs(ctx, h.level, "momo.relation", attrs...)
}
