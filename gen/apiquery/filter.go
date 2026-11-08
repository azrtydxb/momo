// SPDX-License-Identifier: Apache-2.0

package apiquery

import "github.com/azrtydxb/momo"

// Filter is one request filter: a column name, a comparison operator and a
// value as text.
type Filter struct {
	Column string
	// Op is one of eq, ne, lt, le, gt, ge.
	Op    string
	Value string
}

// ParseFilter resolves f.Column through lookup (the generated ColumnByName of
// the model, so only allowlisted columns resolve) and parses f.Value with the
// column's Go type. An unknown column returns the error from lookup; an
// operator outside eq, ne, lt, le, gt and ge, or a value that does not parse,
// returns an error that matches momo.ErrValidation. Values are bound, never
// spliced into SQL.
func ParseFilter(lookup func(name string) (momo.ColumnRef, error), f Filter) (momo.Predicate, error) {
	switch f.Op {
	case "eq", "ne", "lt", "le", "gt", "ge":
	default:
		return nil, invalidf("filter", "unknown filter operator")
	}
	col, err := lookup(f.Column)
	if err != nil {
		return nil, err
	}
	return col.ParseCmp(f.Op, f.Value)
}
