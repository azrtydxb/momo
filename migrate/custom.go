// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"errors"
	"fmt"
)

// customUpTemplate is the body of a new hand-written up file. It holds no
// statement and no -- momo:generated line, so the migration is custom.
const customUpTemplate = `-- Hand-written migration. Write the SQL below; end each statement with a semicolon.
-- Directives (-- momo:no-transaction, -- momo:split, -- momo:destructive) go in
-- this leading comment block, before the first statement.
-- If the SQL changes the schema, change the models too and run momo migrate diff.
-- After editing this file, run: momo migrate hash
`

// NewCustom returns the files of an empty hand-written migration called name:
// the up file, a snapshot that equals the latest one (with the new version and
// that snapshot as its parent) and the extended momo.sum, keyed by relative
// file name like Migration.Files. It reads the directory only. A directory
// without a snapshot is refused, because a custom migration needs a parent to
// copy; run Diff first.
func (e *Engine) NewCustom(name string) (*Migration, error) {
	if !migrationName.MatchString(name) {
		return nil, fmt.Errorf("migrate: migration name %q must match %s", name, migrationName)
	}
	snap, err := LatestSnapshot(e.dir)
	if errors.Is(err, ErrNoSnapshot) {
		return nil, fmt.Errorf("%w: a custom migration copies the latest snapshot; run momo migrate diff first", ErrNoSnapshot)
	}
	if err != nil {
		return nil, err
	}
	version, err := NextVersion(e.opts.clock, e.dir)
	if err != nil {
		return nil, err
	}
	upName := version + "_" + name + ".up.sql"
	snapName := version + "_" + name + ".snapshot.json"
	var snapBuf bytes.Buffer
	if err := WriteSnapshot(&snapBuf, SnapshotFile{Version: version, ParentVersion: snap.Version, Schema: snap.Schema}); err != nil {
		return nil, err
	}
	added := memFS{upName: []byte(customUpTemplate), snapName: snapBuf.Bytes()}
	existing, err := e.existingSum()
	if err != nil {
		return nil, err
	}
	sum, err := AppendSum(existing, added, []string{snapName, upName})
	if err != nil {
		return nil, err
	}
	var sumBuf bytes.Buffer
	if err := sum.Write(&sumBuf); err != nil {
		return nil, err
	}
	return &Migration{
		Version: version, Name: name, Kind: "sql", Parent: snap.Version, Snapshot: snap.Schema,
		Files: map[string][]byte{upName: added[upName], snapName: added[snapName], SumFileName: sumBuf.Bytes()},
	}, nil
}
