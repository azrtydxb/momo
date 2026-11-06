// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// RehashResult reports what Rehash found for one applied version.
type RehashResult struct {
	Version string
	Name    string
	// Recorded is the checksum in the history row; Current is the checksum of
	// the file in the directory.
	Recorded, Current string
	// Changed is true when the two differ, that is, when the file was edited
	// after it was applied.
	Changed bool
	// Accepted is true when the history row now carries Current. It is false
	// when accept was not asked for, and when nothing differed.
	Accepted bool
	// SumStale is true when the file differs from momo.sum, so Status and
	// Apply keep reporting a mismatch until momo migrate hash rewrites it.
	SumStale bool
}

// Rehash compares the checksum recorded in the history row of an applied
// version with the checksum of its file. With accept false it only reports and
// writes nothing. With accept true and a difference it replaces the recorded
// checksum with the file's, on that version's row only, and records the action
// in the row's error column ("checksum accepted by rehash ..."), which Status
// shows. It is the supported way past ErrChecksumMismatch after a deliberate
// edit of an applied migration; momo.sum is a separate file that momo migrate
// hash rewrites.
//
// Rehash holds the migration lock like Apply when it writes. It is an error
// when the version has no applied row or no up file. It does not rerun
// anything: the edit is only accepted, not applied to the database.
func (e *Engine) Rehash(ctx context.Context, version string, accept bool) (res *RehashResult, err error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	if accept {
		end, serr := e.session(ctx, c, e.lockWait(ApplyOptions{}))
		if serr != nil {
			return nil, serr
		}
		defer func() { err = end(err) }()
	}
	hist, err := e.readHistory(ctx, c)
	if err != nil {
		return nil, err
	}
	var row *HistoryRow
	for i := range hist {
		if hist[i].Version == version {
			row = &hist[i]
		}
	}
	if row == nil || row.Status != StatusApplied {
		return nil, fmt.Errorf("migrate: version %s has no applied history row to rehash", version)
	}
	files, err := Discover(e.dir)
	if err != nil {
		return nil, err
	}
	sum, err := ComputeSum(e.dir)
	if err != nil {
		return nil, err
	}
	bad, err := e.sumMismatches()
	if err != nil {
		return nil, err
	}
	cur, path, found := "", "", false
	for _, f := range files {
		if f.Kind == "up" && f.Version == version {
			cur, path, found = sum.Files[f.Path], f.Path, true
		}
	}
	if !found {
		return nil, fmt.Errorf("migrate: version %s has no up file in the migration directory", version)
	}
	res = &RehashResult{Version: version, Name: row.Name, Recorded: row.Checksum, Current: cur, Changed: row.Checksum != cur, SumStale: slices.Contains(bad, path)}
	if !accept || !res.Changed {
		return res, nil
	}
	d := e.opts.dialect
	note := fmt.Sprintf("checksum accepted by rehash at %s: %s -> %s", e.opts.clock().UTC().Format(time.RFC3339), row.Checksum, cur)
	q := fmt.Sprintf(`UPDATE momo_migrations SET checksum = %s, error = %s WHERE version = %s AND status = 'applied' AND checksum = %s`,
		ph(d, 1), ph(d, 2), ph(d, 3), ph(d, 4))
	n, err := c.Exec(ctx, q, []any{cur, note, version, row.Checksum})
	if err != nil {
		return nil, fmt.Errorf("migrate: rehash %s: %w", version, err)
	}
	if n != 1 {
		return nil, fmt.Errorf("migrate: rehash %s: the history row changed while rehashing; run it again", version)
	}
	res.Accepted = true
	return res, nil
}
