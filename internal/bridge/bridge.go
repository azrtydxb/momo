// SPDX-License-Identifier: Apache-2.0

// Package bridge is the registry through which package momo hands its
// extension internals to the feature packages of this module (agg, arrayop,
// fts, jsonop, rangeop, sqlfn, timeop, vecop) without exporting them.
//
// The root package cannot be imported from here, so every value that crosses
// the registry is an any: the builder is the root's *Builder, an expression is
// its Expr. The root registers one Kit from an init function, and
// internal/exprkit wraps the Kit in typed functions that the feature packages
// call. Being under internal/, neither package is importable from outside the
// module, so the registry is not an extension API.
package bridge

import "github.com/azrtydxb/momo/dialect"

// Kit is the set of root-package internals the feature packages need. The
// flags are the bits of momo.ColumnFlags as a plain uint16.
type Kit struct {
	// NewBuilder returns a *momo.Builder for the dialect.
	NewBuilder func(d *dialect.Dialect) any
	// ArgFor binds v on the *momo.Builder, masking it when flags has FlagSensitive.
	ArgFor func(b any, flags uint16, v any)
	// RenderSensitive renders the momo.Expr e and returns flags with
	// FlagSensitive added when e read a sensitive column.
	RenderSensitive func(b, e any, flags uint16) uint16
	// ExprFor renders the momo.Expr e with its arguments masked when flags has
	// FlagSensitive.
	ExprFor func(b any, flags uint16, e any)
	// MarkSensitive records that the expression being rendered read a sensitive
	// column.
	MarkSensitive func(b any)
	// AppendFilter writes " FILTER (WHERE p)" for the momo.Predicate p.
	AppendFilter func(b, p any)
	// Unsupported records an ErrUnsupported build error naming the feature.
	Unsupported func(b any, feature string)
	// IsBound reports whether the momo.Expr e is a bound Go value.
	IsBound func(e any) bool
	// WindowCall returns the momo.Expr "call OVER (w)" for a momo.Expr call and
	// a momo.WindowSpec w.
	WindowCall func(call, w any) any
	// AppendOrder renders the momo.Order o as an ORDER BY item.
	AppendOrder func(b, o any)
	// ValidName and ValidTypeName check an identifier and a SQL type name.
	ValidName     func(s string) error
	ValidTypeName func(s string) error
}

var kit *Kit

// Register installs the root package's Kit. The root calls it once from init.
// It panics naming the first field that is nil, so a function added to Kit but
// not registered fails at start-up instead of at the first call.
func Register(k *Kit) {
	for name, set := range map[string]bool{
		"NewBuilder":      k.NewBuilder != nil,
		"ArgFor":          k.ArgFor != nil,
		"RenderSensitive": k.RenderSensitive != nil,
		"ExprFor":         k.ExprFor != nil,
		"MarkSensitive":   k.MarkSensitive != nil,
		"AppendFilter":    k.AppendFilter != nil,
		"Unsupported":     k.Unsupported != nil,
		"IsBound":         k.IsBound != nil,
		"WindowCall":      k.WindowCall != nil,
		"AppendOrder":     k.AppendOrder != nil,
		"ValidName":       k.ValidName != nil,
		"ValidTypeName":   k.ValidTypeName != nil,
	} {
		if !set {
			panic("bridge: Kit." + name + " is not registered")
		}
	}
	kit = k
}

// Get returns the registered Kit. It is set by the time any code outside the
// root package runs, because every user of the registry imports the root.
func Get() *Kit {
	if kit == nil {
		panic("bridge: the momo package did not register its extension kit")
	}
	return kit
}
