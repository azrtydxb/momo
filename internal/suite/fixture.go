// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Document is the shared integration fixture model, the struct the generator emits for it.
type Document struct {
	ID        int64
	TenantID  int64
	Title     string
	Body      *string    // nullable
	Version   int64      // optimistic lock
	DeletedAt *time.Time // soft delete (timestamp)
	CreatedAt time.Time
	UpdatedAt time.Time
	Secret    string // sensitive
}

// Column descriptors, as generated code would expose them.
var (
	DocID        = momo.NewColumn[int64]("documents", "id", momo.FlagPK|momo.FlagIdentity)
	DocTenantID  = momo.NewColumn[int64]("documents", "tenant_id", 0)
	DocTitle     = momo.NewColumn[string]("documents", "title", 0)
	DocBody      = momo.NewNullColumn[string]("documents", "body", momo.FlagNullable)
	DocStatus    = momo.NewNullColumn[string]("documents", "status", momo.FlagNullable|momo.FlagHasDefault)
	DocVersion   = momo.NewColumn[int64]("documents", "version", momo.FlagVersion)
	DocDeletedAt = momo.NewNullColumn[time.Time]("documents", "deleted_at", momo.FlagSoftDelete|momo.FlagNullable)
	DocCreatedAt = momo.NewColumn[time.Time]("documents", "created_at", momo.FlagCreatedAt)
	DocUpdatedAt = momo.NewColumn[time.Time]("documents", "updated_at", momo.FlagUpdatedAt)
	DocSecret    = momo.NewColumn[string]("documents", "secret", momo.FlagSensitive)
)

// DocumentMeta is the package-level Meta for Document. Table "documents"; columns in order
// id, tenant_id, title, body, version, deleted_at, created_at, updated_at, secret.
// PK []int{0}, Identity 0, Version 4, SoftDelete 5, CreatedAt 6, UpdatedAt 7.
var DocumentMeta = &momo.Meta[Document]{
	Table: "documents",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: DocID.Flags()},
		{Name: "tenant_id", Flags: DocTenantID.Flags()},
		{Name: "title", Flags: DocTitle.Flags()},
		{Name: "body", Flags: DocBody.Flags()},
		{Name: "version", Flags: DocVersion.Flags()},
		{Name: "deleted_at", Flags: DocDeletedAt.Flags()},
		{Name: "created_at", Flags: DocCreatedAt.Flags()},
		{Name: "updated_at", Flags: DocUpdatedAt.Flags()},
		{Name: "secret", Flags: DocSecret.Flags()},
	},
	Ptr: func(m *Document, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.TenantID
		case 2:
			return &m.Title
		case 3:
			return &m.Body
		case 4:
			return &m.Version
		case 5:
			return &m.DeletedAt
		case 6:
			return &m.CreatedAt
		case 7:
			return &m.UpdatedAt
		case 8:
			return &m.Secret
		}
		return nil
	},
	Value: func(m *Document, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.TenantID
		case 2:
			return m.Title
		case 3:
			return m.Body
		case 4:
			return m.Version
		case 5:
			return m.DeletedAt
		case 6:
			return m.CreatedAt
		case 7:
			return m.UpdatedAt
		case 8:
			return m.Secret
		}
		return nil
	},
	TableSchema: &documentsTable,
	Identity:    0,
	Version:     4,
	SoftDelete:  5,
	CreatedAt:   6,
	UpdatedAt:   7,
	PK:          []int{0},
}

// documentsTable is the normalized definition the generator would emit for
// Document: the model's nine columns in field order. The DDL also creates a
// status column that the model does not map, so it is absent here, as it would
// be in generated code.
var documentsTable = schema.Table{
	Name: "documents",
	Columns: []schema.Column{
		{Name: "id", Type: "bigint", Identity: "always"},
		{Name: "tenant_id", Type: "bigint"},
		{Name: "title", Type: "text"},
		{Name: "body", Type: "text", Nullable: true},
		{Name: "version", Type: "bigint", Default: "1"},
		{Name: "deleted_at", Type: "timestamptz", Nullable: true},
		{Name: "created_at", Type: "timestamptz", Default: "now()"},
		{Name: "updated_at", Type: "timestamptz", Default: "now()"},
		{Name: "secret", Type: "text", Default: "''"},
	},
	PrimaryKey: []string{"id"},
}

// MomoMeta returns DocumentMeta.
func (*Document) MomoMeta() *momo.Meta[Document] { return DocumentMeta }

// StatusDocument maps the documents columns the write-state tests need,
// including the status column that Document leaves unmapped. It has no
// timestamp, version or soft-delete column, so its statements bind only what
// the caller writes and every other column takes its database default.
type StatusDocument struct {
	ID       int64
	TenantID int64
	Title    string
	Status   *string
}

// StatusDocumentMeta is the Meta for StatusDocument (table documents).
var StatusDocumentMeta = &momo.Meta[StatusDocument]{
	Table: "documents",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: DocID.Flags()},
		{Name: "tenant_id", Flags: DocTenantID.Flags()},
		{Name: "title", Flags: DocTitle.Flags()},
		{Name: "status", Flags: DocStatus.Flags()},
	},
	Ptr: func(m *StatusDocument, i int) any {
		return [...]any{&m.ID, &m.TenantID, &m.Title, &m.Status}[i]
	},
	Value: func(m *StatusDocument, i int) any {
		return [...]any{m.ID, m.TenantID, m.Title, m.Status}[i]
	},
	Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

// MomoMeta returns StatusDocumentMeta.
func (*StatusDocument) MomoMeta() *momo.Meta[StatusDocument] { return StatusDocumentMeta }

const (
	documentsDDLPostgres = `CREATE TABLE documents (id bigint generated always as identity primary key, tenant_id bigint not null, title text not null, body text, status text default 'draft', version bigint not null default 1, deleted_at timestamptz, created_at timestamptz not null default now(), updated_at timestamptz not null default now(), secret text not null default '')`
	documentsDDLMySQL    = "CREATE TABLE documents (id bigint auto_increment primary key, tenant_id bigint not null, title varchar(255) not null, body text, status varchar(64) default 'draft', version bigint not null default 1, deleted_at datetime(6) null, created_at datetime(6) not null default current_timestamp(6), updated_at datetime(6) not null default current_timestamp(6), secret varchar(255) not null default '')"
	documentsDDLSQLite   = `CREATE TABLE documents (id integer primary key, tenant_id integer not null, title text not null, body text, status text default 'draft', version integer not null default 1, deleted_at text, created_at text not null default (strftime('%Y-%m-%d %H:%M:%f','now')), updated_at text not null default (strftime('%Y-%m-%d %H:%M:%f','now')), secret text not null default '')`
)

// CreateDocuments drops and recreates the documents table with the DDL of the dialect in caps.
func CreateDocuments(t testing.TB, db *momo.DB, caps Caps) {
	t.Helper()
	var ddl string
	switch caps.Dialect {
	case dialect.Postgres:
		ddl = documentsDDLPostgres
	case dialect.SQLite:
		ddl = documentsDDLSQLite
	case dialect.MySQL:
		ddl = documentsDDLMySQL
	default:
		t.Fatalf("suite: no documents DDL for dialect %q", caps.Dialect)
	}
	ctx := context.Background()
	for _, stmt := range []string{"DROP TABLE IF EXISTS documents", ddl} {
		if _, err := db.Driver().Exec(ctx, stmt, nil); err != nil {
			t.Fatalf("suite: %s: %v", stmt, err)
		}
	}
}
