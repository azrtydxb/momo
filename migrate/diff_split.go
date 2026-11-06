// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/azrtydxb/momo/schema"
)

// noTxSuffix names the trailing no-transaction migration after the first one.
const noTxSuffix = "_no_tx"

// splitNoTx divides an ordered plan whose rendering mixes transactional
// statements with statements that must run outside a transaction (CREATE
// INDEX CONCURRENTLY, VALIDATE CONSTRAINT). The transactional part stays in a
// head migration, so a failure rolls it back whole; the other statements go
// to a tail migration that carries -- momo:no-transaction. It reports ok false
// when nothing is left for the head, in which case the whole plan belongs in
// one no-transaction file.
//
// A statement moves to the tail when it is NoTx. Changes that would break if
// they ran before the moved ones follow it:
//   - a later index, unique or foreign-key change on the same name, so a
//     drop and re-create of an index never straddles the two files;
//   - a later foreign key that references a table whose unique index or
//     constraint was moved, which needs that index to exist.
//
// The head's snapshot is the desired schema without the objects the tail
// creates and with the ones it drops, so the chain stays valid between the
// two files.
func (e *Engine) splitNoTx(version, name, parent string, ordered []Change, to *schema.Schema, hints []string) (head, tail *Migration, ok bool, err error) {
	type part struct {
		c          Change
		head, tail []Statement
		whole      bool // every statement of the change is in the tail
	}
	parts := make([]part, len(ordered))
	movedNames := map[string]bool{}  // schema, table, name of moved index-like changes
	movedUnique := map[string]bool{} // tables that gained a unique index in the tail
	for i, c := range ordered {
		stmts, rerr := e.opts.dialect.Render([]Change{c})
		if rerr != nil {
			return nil, nil, false, rerr
		}
		p := part{c: c}
		for _, s := range stmts {
			if s.NoTx {
				p.tail = append(p.tail, s)
			} else {
				p.head = append(p.head, s)
			}
		}
		follow := false
		switch c.Kind {
		case CreateIndex, DropIndex, AddUnique, DropUnique, AddFK, DropFK:
			follow = movedNames[nameKey(c)]
		}
		if c.Kind == AddFK {
			follow = follow || movedUnique[refTableKey(c)]
		}
		if follow && len(p.head) > 0 {
			p.tail = append(p.tail, p.head...)
			p.head = nil
		}
		p.whole = len(p.tail) > 0 && len(p.head) == 0
		if p.whole {
			switch c.Kind {
			case CreateIndex, DropIndex, AddUnique, DropUnique, AddFK, DropFK:
				movedNames[nameKey(c)] = true
			}
			if c.Kind == AddUnique {
				movedUnique[c.Schema+"\x00"+c.Table] = true
			}
			if ix, isIx := c.After.(*schema.Index); c.Kind == CreateIndex && isIx && ix.Unique {
				movedUnique[c.Schema+"\x00"+c.Table] = true
			}
		}
		parts[i] = p
	}

	var headCh, tailCh []Change
	var headStmts, tailStmts []Statement
	for _, p := range parts {
		if len(p.head) > 0 {
			headCh = append(headCh, p.c)
			headStmts = append(headStmts, p.head...)
		}
		if len(p.tail) > 0 {
			tailCh = append(tailCh, p.c)
			tailStmts = append(tailStmts, p.tail...)
		}
	}
	if len(headStmts) == 0 {
		return nil, nil, false, nil
	}

	mid, err := cloneSchema(to)
	if err != nil {
		return nil, nil, false, err
	}
	for _, p := range parts {
		if len(p.tail) > 0 {
			undoInTail(mid, p.c, p.whole)
		}
	}

	t, err := time.Parse(versionLayout, version)
	if err != nil {
		return nil, nil, false, fmt.Errorf("migrate: version %q: %w", version, err)
	}
	head = &Migration{Version: version, Name: name, Kind: "sql", Parent: parent, Up: headStmts, Changes: headCh, Snapshot: mid, Hints: hints}
	tail = &Migration{Version: t.Add(time.Second).Format(versionLayout), Name: name + noTxSuffix, Kind: "sql", Parent: version, Up: tailStmts, Changes: tailCh, Snapshot: to}
	for _, m := range []*Migration{head, tail} {
		finishMigration(m)
	}
	head.Next = tail
	return head, tail, true, nil
}

// finishMigration derives the Destructive and NoTx flags of m from its
// changes and statements.
func finishMigration(m *Migration) {
	m.Destructive, m.NoTx = false, false
	for _, c := range m.Changes {
		m.Destructive = m.Destructive || c.Destructive
	}
	for _, s := range m.Up {
		m.NoTx = m.NoTx || s.NoTx
	}
}

func nameKey(c Change) string { return c.Schema + "\x00" + c.Table + "\x00" + c.Name }

// refTableKey returns the key (schema, NUL, table) of the table a foreign key
// change references, in the spelling of the change's own schema when the
// reference carries none.
func refTableKey(c Change) string {
	fk, ok := c.After.(*schema.ForeignKey)
	if !ok {
		return ""
	}
	sch, tbl := c.Schema, fk.RefTable
	if i := strings.LastIndexByte(tbl, '.'); i >= 0 {
		sch, tbl = tbl[:i], tbl[i+1:]
	}
	return sch + "\x00" + tbl
}

// undoInTail reverts, in the head's snapshot, what the tail does for c: it
// removes created indexes, uniques and foreign keys and restores dropped ones.
// A change that is only partly in the tail (a check added NOT VALID) leaves
// the head with the object, so only whole moves are undone, except for the
// concurrent indexes of a created view, which are always in the tail.
func undoInTail(s *schema.Schema, c Change, whole bool) {
	if c.Kind == CreateView {
		for i := range s.Views {
			if v := &s.Views[i]; v.Schema == c.Schema && v.Name == c.Name {
				v.Indexes = slices.DeleteFunc(v.Indexes, func(ix schema.Index) bool { return ix.Concurrently })
			}
		}
		return
	}
	if !whole {
		return
	}
	var t *schema.Table
	for i := range s.Tables {
		if s.Tables[i].Schema == c.Schema && s.Tables[i].Name == c.Table {
			t = &s.Tables[i]
		}
	}
	if t == nil {
		return
	}
	switch c.Kind {
	case CreateIndex:
		t.Indexes = slices.DeleteFunc(t.Indexes, func(ix schema.Index) bool { return ix.Name == c.Name })
	case AddUnique:
		t.Uniques = slices.DeleteFunc(t.Uniques, func(u schema.Unique) bool { return u.Name == c.Name })
	case AddFK:
		t.ForeignKeys = slices.DeleteFunc(t.ForeignKeys, func(f schema.ForeignKey) bool { return f.Name == c.Name })
	case DropIndex:
		if ix, ok := c.Before.(*schema.Index); ok && !slices.ContainsFunc(t.Indexes, func(o schema.Index) bool { return o.Name == ix.Name }) {
			t.Indexes = append(t.Indexes, *ix)
		}
	case DropUnique:
		if u, ok := c.Before.(*schema.Unique); ok && !slices.ContainsFunc(t.Uniques, func(o schema.Unique) bool { return o.Name == u.Name }) {
			t.Uniques = append(t.Uniques, *u)
		}
	case DropFK:
		if f, ok := c.Before.(*schema.ForeignKey); ok && !slices.ContainsFunc(t.ForeignKeys, func(o schema.ForeignKey) bool { return o.Name == f.Name }) {
			t.ForeignKeys = append(t.ForeignKeys, *f)
		}
	}
}
