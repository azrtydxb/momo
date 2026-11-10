// SPDX-License-Identifier: Apache-2.0

package otelmomo_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/otelmomo"
)

// wantSystem is the db.system.name value for a dialect.
func wantSystem(d dialect.Name) string {
	if d == dialect.Postgres {
		return "postgresql"
	}
	return string(d)
}

func attrs(kvs []attribute.KeyValue) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		m[string(kv.Key)] = kv.Value.String()
	}
	return m
}

func TestOtelSpans(t *testing.T) {
	ctx := context.Background()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() {
		if err := tp.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	h, err := otelmomo.New(tp, sdkmetric.NewMeterProvider(), otelmomo.WithNamespace("appdb"))
	if err != nil {
		t.Fatal(err)
	}
	// WithArgLogging makes argument values available to the hook; the span
	// must still never carry them.
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, db)
		before := len(sr.Ended())
		if _, err := momo.Select[models.Author](db).Where(author.Name.Eq("needle-value")).All(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, `SELECT * FROM missing_table`); err == nil {
			t.Fatal("want an error for a missing table")
		}

		byName := map[string]sdktrace.ReadOnlySpan{}
		for _, s := range sr.Ended()[before:] {
			byName[s.Name()] = s
			for k, v := range attrs(s.Attributes()) {
				if strings.Contains(v, "needle-value") {
					t.Fatalf("span %q attribute %s leaks an argument value", s.Name(), k)
				}
				if k == "db.system" {
					t.Fatalf("span %q uses the obsolete db.system attribute", s.Name())
				}
			}
		}

		sel, ok := byName["SELECT authors"]
		if !ok {
			t.Fatalf("no span named %q; have %v", "SELECT authors", sr.Ended()[before:])
		}
		if sel.SpanKind() != trace.SpanKindClient {
			t.Fatalf("span kind = %v, want client", sel.SpanKind())
		}
		a := attrs(sel.Attributes())
		if a["db.system.name"] != wantSystem(d) || a["db.namespace"] != "appdb" || a["db.collection.name"] != "authors" || a["db.operation.name"] != "SELECT" {
			t.Fatalf("attributes %v", a)
		}
		if !strings.Contains(a["db.query.text"], "FROM") || strings.Contains(a["db.query.text"], "needle") {
			t.Fatalf("db.query.text = %q, want parameterized SQL", a["db.query.text"])
		}
		if sel.Status().Code == codes.Error {
			t.Fatalf("successful span has error status: %+v", sel.Status())
		}

		bad, ok := byName["SELECT"]
		if !ok {
			t.Fatalf("no span named SELECT for the table-less statement; have %v", sr.Ended()[before:])
		}
		if bad.Status().Code != codes.Error {
			t.Fatalf("failed span status = %+v, want Error", bad.Status())
		}
		if attrs(bad.Attributes())["error.type"] == "" {
			t.Fatalf("failed span lacks error.type: %v", attrs(bad.Attributes()))
		}
	}, momo.WithHook(h), momo.WithArgLogging())
}

func TestNewRejectsNilProviders(t *testing.T) {
	if _, err := otelmomo.New(nil, sdkmetric.NewMeterProvider()); err == nil {
		t.Fatal("nil TracerProvider accepted")
	}
	if _, err := otelmomo.New(sdktrace.NewTracerProvider(), nil); err == nil {
		t.Fatal("nil MeterProvider accepted")
	}
}

func TestOtelMetrics(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	h, err := otelmomo.New(sdktrace.NewTracerProvider(), mp, otelmomo.WithNamespace("appdb"))
	if err != nil {
		t.Fatal(err)
	}
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, db)
		if _, err := momo.Select[models.Author](db).Where(author.Name.Eq("x")).All(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, `SELECT * FROM missing_table`); err == nil {
			t.Fatal("want an error")
		}

		var rm metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &rm); err != nil {
			t.Fatal(err)
		}
		var hist metricdata.Metrics
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				if m.Name == "db.client.operation.duration" {
					hist = m
				}
			}
		}
		data, ok := hist.Data.(metricdata.Histogram[float64])
		if !ok {
			t.Fatalf("db.client.operation.duration histogram missing: %+v", rm)
		}
		if hist.Unit != "s" {
			t.Fatalf("unit = %q, want s", hist.Unit)
		}
		wantBounds := []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10}
		var sawSelect, sawError bool
		for _, dp := range data.DataPoints {
			if !slices.Equal(dp.Bounds, wantBounds) {
				t.Fatalf("bucket bounds = %v, want the semantic convention bounds %v", dp.Bounds, wantBounds)
			}
			sys, has := dp.Attributes.Value(attribute.Key("db.system.name"))
			if !has {
				t.Fatalf("data point lacks db.system.name: %v", dp.Attributes)
			}
			if sys.AsString() != wantSystem(d) {
				continue // a data point of another dialect's run
			}
			op, hasOp := dp.Attributes.Value(attribute.Key("db.operation.name"))
			coll, hasColl := dp.Attributes.Value(attribute.Key("db.collection.name"))
			if hasOp && hasColl && op.AsString() == "SELECT" && coll.AsString() == "authors" && dp.Count == 1 && dp.Sum > 0 {
				sawSelect = true
			}
			if _, has := dp.Attributes.Value(attribute.Key("error.type")); has {
				sawError = true
			}
		}
		if !sawSelect || !sawError {
			t.Fatalf("sawSelect=%v sawError=%v in %+v", sawSelect, sawError, data.DataPoints)
		}
	}, momo.WithHook(h))
}
