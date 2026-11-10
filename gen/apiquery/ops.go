// SPDX-License-Identifier: Apache-2.0

package apiquery

import (
	"fmt"
	"slices"
)

// Op is one operation a generated API can serve for a model.
type Op string

// The operations of the generated servers.
const (
	OpList   Op = "list"
	OpGet    Op = "get"
	OpCreate Op = "create"
	OpUpdate Op = "update"
	OpDelete Op = "delete"
)

// AllOps lists every operation, in the order the generators emit them.
var AllOps = []Op{OpList, OpGet, OpCreate, OpUpdate, OpDelete}

// ReadOnlyOps are the operations of a read-only API.
var ReadOnlyOps = []Op{OpList, OpGet}

// Ops chooses the operations a generator serves per model. The key is a model
// name, or DefaultModel for the models not named. A nil Ops, or a model with
// no entry and no default, serves every operation.
type Ops map[string][]string

// DefaultModel is the Ops key that applies to every model without its own entry.
const DefaultModel = "*"

// OpSet is a set of operations.
type OpSet map[Op]bool

// Has reports whether op is in the set.
func (s OpSet) Has(op Op) bool { return s[op] }

// Validate reports an unknown operation name or an empty operation list.
func (o Ops) Validate() error {
	for model, names := range o {
		if len(names) == 0 {
			return fmt.Errorf("ops for %q is empty: list at least one operation, or leave the model out", model)
		}
		for _, n := range names {
			if !slices.Contains(AllOps, Op(n)) {
				return fmt.Errorf("unknown operation %q in ops for %q (want one of list, get, create, update, delete)", n, model)
			}
		}
	}
	return nil
}

// For returns the operations enabled for model.
func (o Ops) For(model string) OpSet {
	names, ok := o[model]
	if !ok {
		names, ok = o[DefaultModel]
	}
	set := OpSet{}
	if !ok {
		for _, op := range AllOps {
			set[op] = true
		}
		return set
	}
	for _, n := range names {
		set[Op(n)] = true
	}
	return set
}
