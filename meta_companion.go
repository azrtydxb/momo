// SPDX-License-Identifier: Apache-2.0

package momo

import "github.com/azrtydxb/momo/schema"

// Config is the receiver of a model's MomoConfig companion method. Every
// method is read statically by the generator from the Go source and is never
// executed for its effect; at runtime they are all no-ops. Field arguments are
// pointers taken from the model receiver (&d.Field); their value is ignored.
type Config struct{}

// IndexOpts carries the optional parts of an index declaration. Its methods
// are read statically by the generator and are no-ops at runtime.
type IndexOpts struct{}

// Index declares a non-unique index over the given field references. Read
// statically by the generator; never executed for its effect.
func (c *Config) Index(name string, cols ...any) *IndexOpts { return &IndexOpts{} }

// IndexExpr declares an expression index. Read statically by the generator;
// never executed for its effect.
func (c *Config) IndexExpr(name string, exprs ...string) *IndexOpts { return &IndexOpts{} }

// Unique declares a unique index over the given field references. Read
// statically by the generator; never executed for its effect.
func (c *Config) Unique(name string, cols ...any) *IndexOpts { return &IndexOpts{} }

// Check declares a named check constraint. Read statically by the generator;
// never executed for its effect.
func (c *Config) Check(name, expr string) {}

// Enum declares a named enum type with its values. Read statically by the
// generator; never executed for its effect.
func (c *Config) Enum(name string, values ...string) {}

// Default sets a column default expression. Read statically by the generator;
// never executed for its effect.
func (c *Config) Default(field any, expr string) {}

// Comment sets a column comment. Read statically by the generator; never
// executed for its effect.
func (c *Config) Comment(field any, text string) {}

// Collate sets a column collation. Read statically by the generator; never
// executed for its effect.
func (c *Config) Collate(field any, collation string) {}

// IDGen declares a Go-side id generator for a string field: an insert fills the
// field when it holds the zero value. The name is a string literal: cuid, uuid
// or ulid, or one registered with WithIDGenerator. Read statically by the
// generator; never executed for its effect.
func (c *Config) IDGen(field any, name string) {}

// FKName names the foreign key constraint of the belongs_to relation whose key
// field is field, for a name other than the derived <table>_<column>_fkey (for
// example _fkey1 on a partitioned table). Read statically by the generator;
// never executed for its effect.
func (c *Config) FKName(field any, name string) {}

// PrimaryKey orders the columns of a composite primary key, for a key whose
// column order differs from the field order (the order is the btree order that
// lookups rely on). The fields must be exactly the fields tagged pk. Read
// statically by the generator; never executed for its effect.
func (c *Config) PrimaryKey(fields ...any) {}

// View declares a view or materialized view. The argument must be a
// composite literal; read statically by the generator, never executed.
func (c *Config) View(v schema.View) {}

// Function declares a function. The argument must be a composite literal;
// read statically by the generator, never executed.
func (c *Config) Function(f schema.Function) {}

// Trigger declares a trigger. The argument must be a composite literal; read
// statically by the generator, never executed.
func (c *Config) Trigger(t schema.Trigger) {}

// Policy declares a row-level security policy. The argument must be a
// composite literal; read statically by the generator, never executed.
func (c *Config) Policy(p schema.Policy) {}

// RLS declares the table's row-level security flags. The argument must be a
// composite literal; read statically by the generator, never executed.
func (c *Config) RLS(r schema.RLS) {}

// Partition declares the table's partitioning. The argument must be a
// composite literal; read statically by the generator, never executed.
func (c *Config) Partition(p schema.Partition) {}

// PartitionOf declares the table as a partition of a parent. The argument
// must be a composite literal; read statically by the generator, never
// executed.
func (c *Config) PartitionOf(p schema.PartitionOf) {}

// Where sets the partial-index predicate. Read statically; no-op at runtime.
func (o *IndexOpts) Where(pred string) *IndexOpts { return o }

// Method sets the index access method. Read statically; no-op at runtime.
func (o *IndexOpts) Method(m string) *IndexOpts { return o }

// With sets one index storage parameter. Read statically; no-op at runtime.
func (o *IndexOpts) With(key, value string) *IndexOpts { return o }

// Opclass sets the operator class of one indexed field. Read statically;
// no-op at runtime.
func (o *IndexOpts) Opclass(field any, opclass string) *IndexOpts { return o }

// Desc marks one indexed field descending. Read statically; no-op at runtime.
func (o *IndexOpts) Desc(field any) *IndexOpts { return o }

// Nulls sets NULLS ordering ("first" or "last") for one indexed field. Read
// statically; no-op at runtime.
func (o *IndexOpts) Nulls(field any, order string) *IndexOpts { return o }

// Concurrently marks the index to be built concurrently. Read statically;
// no-op at runtime.
func (o *IndexOpts) Concurrently() *IndexOpts { return o }
