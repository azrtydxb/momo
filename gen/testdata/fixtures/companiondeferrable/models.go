// SPDX-License-Identifier: Apache-2.0

// Package companiondeferrable declares deferrable constraints in MomoConfig.
package companiondeferrable

import "github.com/azrtydxb/momo"

type Stop struct {
	momo.Model `momo:"table:stops"`
	ID         int64 `momo:"pk"`
	RouteID    int64
	Position   int64
}

func (s *Stop) MomoConfig(c *momo.Config) {
	c.Unique("stops_route_position_key", &s.RouteID, &s.Position).InitiallyDeferred()
	c.Unique("stops_route_key", &s.RouteID).Deferrable()
	c.Unique("stops_position_key", &s.Position)
	c.ForeignKey("stops_route_id_fkey", []any{&s.RouteID}, "routes", []string{"id"}).OnDelete("CASCADE").Deferrable()
	c.PrimaryKeyDeferrable()
}

type Route struct {
	momo.Model `momo:"table:routes"`
	ID         int64 `momo:"pk"`
	Name       string
}

// Depot has a deferrable primary key; no foreign key references it, because
// PostgreSQL cannot use a deferrable key as a foreign key target.
type Depot struct {
	momo.Model `momo:"table:depots"`
	ID         int64 `momo:"pk"`
	Name       string
}

func (d *Depot) MomoConfig(c *momo.Config) {
	c.PrimaryKeyInitiallyDeferred()
}
