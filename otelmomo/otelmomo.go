// SPDX-License-Identifier: Apache-2.0

package otelmomo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/semconv/v1.43.0/dbconv"
	"go.opentelemetry.io/otel/trace"

	"github.com/azrtydxb/momo"
)

const scope = "github.com/azrtydxb/momo/otelmomo"

type config struct{ namespace string }

// Option configures New.
type Option func(*config)

// WithNamespace sets db.namespace, the database name, on every span and
// measurement.
func WithNamespace(ns string) Option { return func(c *config) { c.namespace = ns } }

type hook struct {
	tracer trace.Tracer
	ns     string
	dur    dbconv.ClientOperationDuration
}

var _ momo.Hook = (*hook)(nil)

// New returns a momo.Hook that records one client span and one
// db.client.operation.duration measurement per statement. The providers are
// passed in; the package uses no global provider, and a nil provider is an
// error. SQL text is recorded as db.query.text; argument values never are.
func New(tp trace.TracerProvider, mp metric.MeterProvider, opts ...Option) (momo.Hook, error) {
	if tp == nil {
		return nil, errors.New("otelmomo: nil TracerProvider")
	}
	if mp == nil {
		return nil, errors.New("otelmomo: nil MeterProvider")
	}
	var c config
	for _, o := range opts {
		o(&c)
	}
	dur, err := dbconv.NewClientOperationDuration(mp.Meter(scope))
	if err != nil {
		return nil, fmt.Errorf("otelmomo: create db.client.operation.duration: %w", err)
	}
	return &hook{tracer: tp.Tracer(scope), ns: c.namespace, dur: dur}, nil
}

// operationName returns the first keyword of sql, upper-cased.
func operationName(sql string) string {
	s := strings.TrimLeft(sql, " \t\r\n(")
	if i := strings.IndexAny(s, " \t\r\n("); i >= 0 {
		s = s[:i]
	}
	return strings.ToUpper(s)
}

// BeforeQuery starts the span and returns the context that carries it.
func (h *hook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	op := operationName(e.SQL)
	name := op
	attrs := []attribute.KeyValue{
		semconv.DBSystemNameKey.String(string(systemName(e.Dialect))),
		semconv.DBOperationName(op),
		semconv.DBQueryText(e.SQL),
	}
	if e.Table != "" {
		name = op + " " + e.Table
		attrs = append(attrs, semconv.DBCollectionName(e.Table))
	}
	if h.ns != "" {
		attrs = append(attrs, semconv.DBNamespace(h.ns))
	}
	ctx, _ = h.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	return ctx
}

// errorType returns the low-cardinality error.type value for err.
func errorType(err error) string {
	var me *momo.Error
	if errors.As(err, &me) && me.Kind != nil {
		return me.Kind.Error()
	}
	return fmt.Sprintf("%T", err)
}

// AfterQuery ends the span and records the duration.
func (h *hook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	span := trace.SpanFromContext(ctx)
	attrs := []attribute.KeyValue{semconv.DBOperationName(operationName(e.SQL))}
	if e.Table != "" {
		attrs = append(attrs, semconv.DBCollectionName(e.Table))
	}
	if h.ns != "" {
		attrs = append(attrs, semconv.DBNamespace(h.ns))
	}
	if e.Err != nil {
		et := errorType(e.Err)
		attrs = append(attrs, semconv.ErrorTypeKey.String(et))
		span.SetAttributes(semconv.ErrorTypeKey.String(et))
		span.SetStatus(codes.Error, et)
	}
	h.dur.Record(ctx, e.Duration.Seconds(), systemName(e.Dialect), attrs...)
	span.End()
}
