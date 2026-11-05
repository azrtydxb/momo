// SPDX-License-Identifier: Apache-2.0

// Package suite holds the integration fixtures and the feature suites that
// every dialect's tests run. Suite functions take a *momo.DB and a Caps value
// and assert ErrUnsupported for every capability that is false; they never skip.
package suite

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Caps describes what a dialect and driver support. Suite functions assert ErrUnsupported for
// every capability that is false; they never skip.
type Caps struct {
	Dialect             dialect.Name
	Returning           bool
	Locking             bool
	DefaultInUpdate     bool // UPDATE ... SET col = DEFAULT
	DefaultInValues     bool // INSERT ... VALUES (DEFAULT)
	Merge               bool
	PartialIndexes      bool // CREATE UNIQUE INDEX ... WHERE
	WindowFunctions     bool
	JSON                bool // JSONB operators
	Arrays              bool // array operators
	Copy                bool // driver implements momo.Copier
	Batch               bool // driver implements momo.Batcher
	NonDefaultIsolation bool // dialect.IsolationLevels
	Vectors             bool // driver implements momo.CapabilityReporter with Vectors true
}

// CapabilitiesOf returns the dialect-level capabilities. Driver-level fields (Copy, Batch, Vectors)
// stay false; CapsOf adds them from the driver.
//
// TODO(momo-05 Task 24): replace the name-derived JSON and Arrays below with
// the dialect flags once that task adds them.
//
// JSON and Arrays are PostgreSQL-only (JSONB and array operators). The dialect
// flags that will carry them arrive with the JSON and array operator task; until
// then they are derived from the dialect name.
func CapabilitiesOf(d *dialect.Dialect) Caps {
	return Caps{
		Dialect:             d.Name,
		Returning:           d.Returning,
		Locking:             d.Locking,
		DefaultInUpdate:     d.DefaultInUpdate,
		DefaultInValues:     d.DefaultInValues,
		Merge:               d.Merge,
		PartialIndexes:      d.Name != dialect.MySQL,
		WindowFunctions:     d.WindowFunctions,
		JSON:                d.Name == dialect.Postgres,
		Arrays:              d.Name == dialect.Postgres,
		NonDefaultIsolation: d.IsolationLevels,
	}
}

// CapsOf returns CapabilitiesOf(db.Dialect()) plus the driver-level capabilities of db.Driver():
// Copy from momo.Copier, Batch from momo.Batcher, Vectors from momo.CapabilityReporter.
func CapsOf(db *momo.DB) Caps {
	c := CapabilitiesOf(db.Dialect())
	drv := db.Driver()
	_, c.Copy = drv.(momo.Copier)
	_, c.Batch = drv.(momo.Batcher)
	if r, ok := drv.(momo.CapabilityReporter); ok {
		c.Vectors = r.Capabilities().Vectors
	}
	return c
}
