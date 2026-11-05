// SPDX-License-Identifier: Apache-2.0

package aliasembed

import "github.com/azrtydxb/momo"

type M = momo.Model

type ViaAlias struct {
	M
	ID int64
}

type ViaPtrAlias struct {
	*M
	ID int64
}

type X momo.Model

type ViaDefined struct {
	X
	ID int64
}

type Shared struct {
	momo.Model
	ID   int64
	E, F int `momo:"bogus_option"`
}
