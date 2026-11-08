// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"fmt"

	"github.com/azrtydxb/momo/schema"
)

// AutoMigrate brings the live database up to desired by additive changes only
// and reports every change it left alone. It is meant for development and
// tests, and for apps that accept additive-only automatic migration; it writes
// no history rows and no migration files.
//
// It takes the migration lock (the one Apply takes, waiting up to WithLockWait
// or 30s), so instances that start together run one after another; each one
// inspects the database after it holds the lock and therefore sees the work the
// previous one did. It then compares the inspected database with desired and
// applies only: create_schema, create_extension, create_enum, create_table,
// add_column that is nullable or has a default or identity, and create_index.
// A new table brings its own foreign keys, row-level security and policies
// with it. Everything else is returned in AutoReport.Skipped with a reason:
// drops, renames, alter_column (type, nullability, default), a NOT NULL column
// without default, a unique index on an existing table, foreign keys, checks
// and primary keys of existing tables, and views, functions and triggers. It
// never emits DROP or ALTER ... TYPE. The kept changes run in one transaction
// when the dialect has transactional DDL and no statement needs to run outside
// one (CREATE INDEX CONCURRENTLY); otherwise they run one by one.
//
// A kept change never depends on a skipped one: an index on a column whose
// add_column or type change was skipped, a foreign key, policy or row-level
// security on a table whose creation was skipped, or a column typed by a
// skipped enum is skipped as well, with the reason "depends on a skipped
// change: <change>". An add_column that would rewrite or scan the table (a
// stored generated column, an identity column, or a default that is not a
// constant literal, NULL, now() or CURRENT_TIMESTAMP) is skipped too.
//
// A non-unique index on an existing table is created in the same run. It is
// built CONCURRENTLY, without blocking writes, only when the schema index sets
// Concurrently; otherwise CREATE INDEX holds a lock that blocks writes to the
// table for the duration of the build.
//
// AutoMigrate ignores migration history entirely: it neither reads nor checks
// the history table, so it runs even when the versioned history is dirty or its
// checksums no longer match. Mixing AutoMigrate with versioned migrations on
// the same schema is the caller's responsibility.
func (e *Engine) AutoMigrate(ctx context.Context, desired *schema.Schema) (rep *AutoReport, err error) {
	if desired == nil {
		return nil, errors.New("migrate: AutoMigrate needs a desired schema")
	}
	to := *desired
	if to.Dialect == "" {
		to.Dialect = e.opts.dialect.Dialect().Name
	}
	to = *dropExternalExtensions(&to, e.opts.config.ExternalExtensions)
	if v, ok := e.opts.dialect.(SchemaValidator); ok {
		if err := v.ValidateSchema(&to); err != nil {
			return nil, err
		}
	}
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	end, err := e.session(ctx, c, e.lockWait(ApplyOptions{}))
	if err != nil {
		return nil, err
	}
	defer func() { err = end(err) }()

	live, _, err := e.inspectOn(ctx, c)
	if err != nil {
		return nil, err
	}
	// Destructive and staged changes are requested so that Diff reports them
	// instead of refusing; the filter below decides what runs.
	changes, _, err := Diff(live, &to, DiffOptions{AllowDestructive: true, Stage: true})
	if err != nil {
		return nil, err
	}
	rep = &AutoReport{}
	created := map[string]bool{}
	for _, ch := range changes {
		if ch.Kind == CreateTable {
			created[ch.Schema+"\x00"+ch.Name] = true
		}
	}
	var kept []Change
	for _, ch := range changes {
		if reason := autoSkipReason(ch, &to, created); reason != "" {
			rep.Skipped = append(rep.Skipped, Skipped{Change: ch, Reason: reason})
			continue
		}
		kept = append(kept, ch)
	}
	kept, rep.Skipped = skipDependents(kept, rep.Skipped)
	if len(kept) == 0 {
		return rep, nil
	}
	ordered, err := OrderErr(kept)
	if err != nil {
		return nil, fmt.Errorf("migrate: order changes: %w", err)
	}
	stmts, err := e.opts.dialect.Render(ordered)
	if err != nil {
		return nil, err
	}
	if e.opts.dialect.Dialect().TransactionalDDL && !hasNoTx(stmts) {
		err = RunStatements(ctx, c, stmts)
	} else {
		_, err = execAll(ctx, c, stmts, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("migrate: AutoMigrate: %w", e.classify(err))
	}
	rep.Applied = ordered
	return rep, nil
}

// autoSkipReason returns why AutoMigrate must not apply ch, or "" when it may.
// created holds the keys (schema, NUL, name) of the tables this run creates.
func autoSkipReason(ch Change, desired *schema.Schema, created map[string]bool) string {
	newTable := created[ch.Schema+"\x00"+ch.Table]
	switch ch.Kind {
	case CreateSchema, CreateExtension, CreateEnum, CreateTable:
		return ""
	case AddColumn:
		// Diff with Stage turns a NOT NULL column without default into a
		// nullable add, so the declared column decides.
		if col := declaredColumn(desired, ch); col != nil {
			if needsStaging(col) {
				return "NOT NULL column without a default requires a staged migration"
			}
			return addColumnRewriteReason(col)
		}
		return ""
	case CreateIndex:
		if ix, ok := ch.After.(*schema.Index); ok && ix.Unique && !newTable {
			return "unique index on an existing table requires a migration"
		}
		return ""
	case AddFK, SetRLS, CreatePolicy:
		if newTable {
			return ""
		}
		return "change to an existing table requires a migration"
	case AlterColumn:
		if ch.Destructive {
			return "type change requires a migration"
		}
		return "column change requires a migration"
	case DropColumn:
		return "column removed in models; AutoMigrate never drops"
	case DropTable:
		return "table removed in models; AutoMigrate never drops"
	case DropEnum, DropIndex, DropUnique, DropCheck, DropFK, DropPrimaryKey, DropView, DropFunction, DropTrigger, DropPolicy:
		return "object removed in models; AutoMigrate never drops"
	case RenameTable, RenameColumn:
		return "rename requires a migration"
	}
	return "change requires a migration"
}

// declaredColumn returns the column of the desired schema that an add_column
// change adds, or nil.
func declaredColumn(desired *schema.Schema, ch Change) *schema.Column {
	for _, t := range desired.Tables {
		if t.Schema == ch.Schema && t.Name == ch.Table {
			for i := range t.Columns {
				if t.Columns[i].Name == ch.Name {
					return &t.Columns[i]
				}
			}
		}
	}
	return nil
}
