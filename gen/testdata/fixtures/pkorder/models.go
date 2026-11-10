// SPDX-License-Identifier: Apache-2.0

package pkorder

import (
	"time"

	"github.com/azrtydxb/momo"
)

// HealthSample declares the primary key (name, scope, at) in an order that
// differs from its field order.
type HealthSample struct {
	momo.Model
	At    time.Time `momo:"pk"`
	Scope string    `momo:"pk"`
	Name  string    `momo:"pk"`
	Value float64
}

func (h *HealthSample) MomoConfig(c *momo.Config) {
	c.PrimaryKey(&h.Name, &h.Scope, &h.At)
}
