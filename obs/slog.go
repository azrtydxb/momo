// SPDX-License-Identifier: Apache-2.0

package obs

import (
	"context"
	"log/slog"
	"time"

	"github.com/azrtydxb/momo"
)

// Slog is a momo.Hook and momo.RelationHook that logs through log/slog. It is
// safe for concurrent use when the logger is, which every slog.Handler must be.
type Slog struct {
	l      *slog.Logger
	level  slog.Level
	slow   time.Duration
	values bool
}

var (
	_ momo.Hook         = (*Slog)(nil)
	_ momo.RelationHook = (*Slog)(nil)
)

// SlogOption configures NewSlog.
type SlogOption func(*Slog)

// SlogLevel sets the level for ordinary records (default Debug).
func SlogLevel(l slog.Level) SlogOption { return func(s *Slog) { s.level = l } }

// SlogSlowOver logs statements slower than d at Warn.
func SlogSlowOver(d time.Duration) SlogOption { return func(s *Slog) { s.slow = d } }

// SlogArgs includes the bound argument values in the statement record under
// the attribute args. It has an effect only when the DB was also opened with
// momo.WithArgLogging, because otherwise the event carries no values. The
// values are those the DB hands to hooks: a value bound from a sensitive column
// is momo.RedactedArg, never the real value.
func SlogArgs(on bool) SlogOption { return func(s *Slog) { s.values = on } }

// NewSlog returns a hook that logs one "momo.query" record per finished
// statement with the attributes op, table, relation (when set), sql, duration,
// rows, err (when set) and, with SlogArgs, args. It also logs one
// "momo.relation" record per finished relation load with parent, name, kind,
// parents, children, queries, duration and err (when set).
func NewSlog(l *slog.Logger, opts ...SlogOption) *Slog {
	s := &Slog{l: l, level: slog.LevelDebug}
	for _, o := range opts {
		o(s)
	}
	return s
}

// BeforeQuery does nothing.
func (s *Slog) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context { return ctx }

// AfterQuery logs the finished statement.
func (s *Slog) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	level := s.level
	if s.slow > 0 && e.Duration > s.slow {
		level = slog.LevelWarn
	}
	if !s.l.Enabled(ctx, level) {
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
	if s.values && e.Args != nil {
		attrs = append(attrs, slog.Any("args", e.Args))
	}
	if e.Err != nil {
		attrs = append(attrs, slog.String("err", e.Err.Error()))
	}
	s.l.LogAttrs(ctx, level, "momo.query", attrs...)
}

// BeforeRelation does nothing.
func (s *Slog) BeforeRelation(ctx context.Context, _ *momo.RelationEvent) context.Context {
	return ctx
}

// AfterRelation logs the finished relation load.
func (s *Slog) AfterRelation(ctx context.Context, e *momo.RelationEvent) {
	if !s.l.Enabled(ctx, s.level) {
		return
	}
	attrs := []slog.Attr{
		slog.String("parent", e.Parent),
		slog.String("name", e.Name),
		slog.Any("kind", e.Kind),
		slog.Int("parents", e.Parents),
		slog.Int64("children", e.Children),
		slog.Int("queries", e.Queries),
		slog.Duration("duration", e.Duration),
	}
	if e.Err != nil {
		attrs = append(attrs, slog.String("err", e.Err.Error()))
	}
	s.l.LogAttrs(ctx, s.level, "momo.relation", attrs...)
}
