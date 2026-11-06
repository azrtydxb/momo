// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

// BaselineApplyID is the apply_id of the history rows Baseline writes.
const BaselineApplyID = "baseline"

// Baseline records every migration up to and including version as applied
// without running it, for a database whose objects were created outside the
// migration history. It executes no DDL against the application schema.
//
// It first checks that the live database matches the snapshot of version
// (ErrNoSnapshot when there is none). A mismatch is an error wrapping
// ErrBaselineMismatch that lists the differing object kinds, unless force is
// set. An index PostgreSQL marks invalid counts as a mismatch. Before it compares,
// a dialect that implements Canonicalizer rewrites the snapshot's declared view,
// function, policy and trigger text to what the database reports, so declared
// text that PostgreSQL spells differently does not count as a difference.
//
// Canonicalizing creates a scratch schema, which on PostgreSQL needs the CREATE
// privilege on the database. A role without it gets an error wrapping
// ErrCanonicalizePrivilege naming that privilege; run the baseline with a role
// that has it. Canonicalization is never skipped, because raw declared text
// would give false mismatches or false matches, and force does not bypass this
// requirement. Only a snapshot without views, functions, triggers and policies
// needs no canonicalization: then no DDL runs and no privilege is required.
//
// For every migration with a version not after version that has no history row
// (a rolled_back row counts as none) it inserts a row with status applied,
// statements_applied 0, the checksum of the file and apply_id "baseline", in one
// transaction. It takes the migration lock and refuses, like Apply, while a
// migration is dirty or a file differs from its checksum. The snapshot files are
// not rewritten; a later Diff compares the models with the snapshot as written.
func (e *Engine) Baseline(ctx context.Context, version string, force bool) (err error) {
	snap, err := snapshotAt(e.dir, version)
	if err != nil {
		return err
	}
	c, err := e.pin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Release() }()
	end, err := e.session(ctx, c, e.lockWait(ApplyOptions{}))
	if err != nil {
		return err
	}
	defer func() { err = end(err) }()

	hist, err := e.history(ctx, c)
	if err != nil {
		return err
	}
	if err := dirtyError(hist); err != nil {
		return err
	}
	if err := e.verifyChecksums(hist); err != nil {
		return err
	}
	live, invalid, err := e.inspectOn(ctx, c)
	if err != nil {
		return err
	}
	want, err := cloneSchema(applyExclude(snap.Schema, e.opts.config.Exclude))
	if err != nil {
		return err
	}
	if cz, ok := e.opts.dialect.(Canonicalizer); ok {
		if err := cz.Canonicalize(ctx, c, want); err != nil {
			return fmt.Errorf("migrate: canonicalize snapshot %s: %w", version, err)
		}
	}
	if rep := compare(want, live, true, invalid); !rep.Clean() && !force {
		return fmt.Errorf("%w: differing object kinds: %s (%d changed, %d missing, %d unmanaged); fix the database, adjust Config.Exclude, or force the baseline",
			ErrBaselineMismatch, strings.Join(rep.kinds(), ", "), len(rep.Changed), len(rep.Missing), len(rep.Unmanaged))
	}
	return e.insertBaselineRows(ctx, c, version, hist)
}

// cloneSchema returns a deep copy of s through its JSON form.
func cloneSchema(s *schema.Schema) (*schema.Schema, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("migrate: copy snapshot: %w", err)
	}
	var out schema.Schema
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("migrate: copy snapshot: %w", err)
	}
	return &out, nil
}

// insertBaselineRows writes the applied rows of every migration up to version
// that the history does not already hold, in one transaction.
func (e *Engine) insertBaselineRows(ctx context.Context, c momo.PinnedConn, version string, hist []HistoryRow) (err error) {
	have := map[string]bool{}
	for _, r := range hist {
		if r.Status != StatusRolledBack {
			have[r.Version] = true
		}
	}
	files, err := Discover(e.dir)
	if err != nil {
		return err
	}
	sum, err := ComputeSum(e.dir)
	if err != nil {
		return err
	}
	tx, err := c.Begin(ctx, momo.TxOptions{})
	if err != nil {
		return fmt.Errorf("migrate: begin: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
	}()
	w := historyWriter{d: e.opts.dialect}
	now := e.opts.clock().UTC()
	for _, f := range files {
		if f.Kind != "up" || f.Version > version || have[f.Version] {
			continue
		}
		if err := w.deleteRolledBack(ctx, tx, f.Version); err != nil {
			return err
		}
		row := HistoryRow{
			Version: f.Version, Name: f.Name, Checksum: sum.Files[f.Path], Status: StatusApplied,
			ApplyID: BaselineApplyID, Kind: "sql", AppliedAt: now.Truncate(time.Microsecond),
		}
		if err := w.insert(ctx, tx, row); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit: %w", err)
	}
	return nil
}
