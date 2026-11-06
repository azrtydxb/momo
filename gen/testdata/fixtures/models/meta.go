// SPDX-License-Identifier: Apache-2.0

package models

import "github.com/azrtydxb/momo"

// The relation descriptors in the descriptor golden call MomoMeta, so the
// golden package builds.

func (*Tenant) MomoMeta() *momo.Meta[Tenant]     { return &momo.Meta[Tenant]{} }
func (*Document) MomoMeta() *momo.Meta[Document] { return &momo.Meta[Document]{} }
func (*Profile) MomoMeta() *momo.Meta[Profile]   { return &momo.Meta[Profile]{} }
func (*Comment) MomoMeta() *momo.Meta[Comment]   { return &momo.Meta[Comment]{} }
func (*Tag) MomoMeta() *momo.Meta[Tag]           { return &momo.Meta[Tag]{} }
