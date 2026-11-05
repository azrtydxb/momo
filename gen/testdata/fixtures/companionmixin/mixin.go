// SPDX-License-Identifier: Apache-2.0

package companionmixin

import "github.com/azrtydxb/momo"

type Coords struct {
	Lat float64
	Lon float64
}

type Place struct {
	momo.Model `momo:"table:places"`
	ID         int64
	Coords
	Home Coords `momo:"embedded,prefix:home_"`
}

func (p *Place) MomoConfig(c *momo.Config) {
	c.Index("places_lon_promoted", &p.Lon)
	c.Index("places_lon_path", &p.Coords.Lon)
	c.Index("places_home_lat", &p.Home.Lat)
	c.Default(&p.Coords.Lat, "0")
}

type ByValue struct {
	momo.Model `momo:"table:by_values"`
	ID         int64
	Name       string
}

func (b ByValue) MomoConfig(c *momo.Config) {
	c.Index("by_values_name_idx", &b.Name)
}
