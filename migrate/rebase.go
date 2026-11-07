// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/azrtydxb/momo/schema"
)

// Rebase regenerates this branch's migrations that are absent from target on
// top of the target's latest snapshot. target is the directory of the target
// branch's migrations; the caller produces it (a git checkout, a worktree),
// Momo has no git dependency.
//
// A branch migration is one of the engine directory's versions that target
// does not have. Every one of them must carry the -- momo:generated header,
// because only generated SQL can be regenerated: Rebase refuses with an error
// naming the file when one is hand-written, and changes nothing.
//
// removed lists, sorted, every file of the branch migrations (up, down and
// snapshot) that the caller should delete. mig is the single new migration
// that takes the target's latest snapshot to desired, with the files to write
// next to target's own (its momo.sum covers target plus the new files); it is
// nil when target already matches desired. Rebase reads the directories only
// and does not touch the database or write files. Its diff uses the default
// DiffOptions, so a destructive or staged change needs a hand-run diff.
func (e *Engine) Rebase(ctx context.Context, target fs.FS, desired *schema.Schema, name string) (removed []string, mig *Migration, err error) {
	if target == nil {
		return nil, nil, errors.New("migrate: Rebase needs a target directory")
	}
	branch, err := Discover(e.dir)
	if err != nil {
		return nil, nil, err
	}
	tfiles, err := Discover(target)
	if err != nil {
		return nil, nil, fmt.Errorf("migrate: target: %w", err)
	}
	inTarget := map[string]bool{}
	for _, f := range tfiles {
		inTarget[f.Version] = true
	}
	mine := map[string]bool{}
	for _, f := range branch {
		if f.Kind == "up" && !inTarget[f.Version] {
			dirs, err := ParseDirectives(f.Body)
			if err != nil {
				return nil, nil, fmt.Errorf("migrate: %s: %w", f.Path, err)
			}
			if !dirs.Generated {
				return nil, nil, fmt.Errorf("migrate: %s has no -- momo:generated header, so it cannot be regenerated; move its statements into a new migration by hand and delete it before rebasing", f.Path)
			}
			mine[f.Version] = true
		}
	}
	for _, f := range branch {
		if mine[f.Version] {
			removed = append(removed, f.Path)
		}
	}
	slices.Sort(removed)
	onTarget := &Engine{drv: e.drv, dir: target, opts: e.opts}
	mig, err = onTarget.Diff(ctx, desired, name, DiffOptions{})
	if err != nil {
		return nil, nil, err
	}
	return removed, mig, nil
}
