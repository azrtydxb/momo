// SPDX-License-Identifier: Apache-2.0

// Package otelmomo is a momo.Hook that emits OpenTelemetry spans and metrics
// for every statement, following the OpenTelemetry database client semantic
// conventions (db.system.name, db.query.text, db.client.operation.duration).
//
// It is a module of its own, github.com/azrtydxb/momo/otelmomo, so the Momo
// core module imports no OpenTelemetry package: only a program that imports
// otelmomo links them. New takes the tracer and meter providers explicitly and
// the package uses no global provider.
//
//	h, err := otelmomo.New(tp, mp, otelmomo.WithNamespace("appdb"))
//	if err != nil {
//		return err
//	}
//	db := momo.New(drv, momo.WithHook(h))
//
// A span is a client span named "<OPERATION> <table>" (or "<OPERATION>" when
// the statement has no known table). Its db.query.text is the parameterized SQL
// as the driver received it; the value of a bound argument is never recorded,
// not even with momo.WithArgLogging. A failed statement sets the span status to
// error and the error.type attribute.
package otelmomo
