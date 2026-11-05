// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"time"

	"github.com/azrtydxb/momo"
)

// unitDocument is the one hand-written model fixture of the root unit tests,
// shaped like generated code.
type unitDocument struct {
	ID        int64
	TenantID  int64
	Title     string
	Published bool
	DeletedAt *time.Time
}

var (
	DocID        = momo.NewColumn[int64]("documents", "id", momo.FlagPK|momo.FlagIdentity)
	DocTenantID  = momo.NewColumn[int64]("documents", "tenant_id", 0)
	DocTitle     = momo.NewColumn[string]("documents", "title", 0)
	DocPublished = momo.NewColumn[bool]("documents", "published", 0)
	DocDeletedAt = momo.NewNullColumn[time.Time]("documents", "deleted_at", momo.FlagNullable|momo.FlagSoftDelete)
)

var unitDocumentMeta = momo.Meta[unitDocument]{
	Table: "documents",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: DocID.Flags()},
		{Name: "tenant_id", Flags: DocTenantID.Flags()},
		{Name: "title", Flags: DocTitle.Flags()},
		{Name: "published", Flags: DocPublished.Flags()},
		{Name: "deleted_at", Flags: DocDeletedAt.Flags()},
	},
	Ptr: func(m *unitDocument, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.TenantID
		case 2:
			return &m.Title
		case 3:
			return &m.Published
		case 4:
			return &m.DeletedAt
		}
		return nil
	},
	Value: func(m *unitDocument, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.TenantID
		case 2:
			return m.Title
		case 3:
			return m.Published
		case 4:
			return m.DeletedAt
		}
		return nil
	},
	Identity:   0,
	Version:    -1,
	SoftDelete: 4,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

// MomoMeta ties unitDocument to its metadata.
func (*unitDocument) MomoMeta() *momo.Meta[unitDocument] { return &unitDocumentMeta }
