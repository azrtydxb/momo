// SPDX-License-Identifier: Apache-2.0

package embedmore

import "github.com/azrtydxb/momo"

type Geo struct {
	Lat float64 `momo:"column:lat"`
	Lon float64 `momo:"column:lon,unique:geo_lon_u"`
}

type Place struct {
	Name string
	Geo  Geo `momo:"embedded,prefix:geo_"`
}

type Aliased = Place

type Venue struct {
	momo.Model `momo:"table:venues"`
	ID         int64 `momo:"pk"`
	Aliased
	Origin Place `momo:"embedded,prefix:o_"`
	Skip   Geo   `momo:"-"`
}
