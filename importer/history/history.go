// SPDX-License-Identifier: Apache-2.0

// Package history converts the migration history of another tool (Atlas,
// goose, golang-migrate, Bun) into a Momo migration directory plus the
// history rows that mark those migrations as applied, so none of them runs
// again. Tool-specific readers produce a Source; this package does the rest.
package history

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/importer"
	"github.com/azrtydxb/momo/migrate"
)

// MappedBase is the version a source version that is not a timestamp is
// mapped after: the migration at 1-based position n in the source list (all
// migrations count, timestamped ones too) gets MappedBase plus n seconds.
const MappedBase = "19700101000000"

// ErrSourceDirty is returned when the source tool recorded a half-applied
// migration.
var ErrSourceDirty = errors.New("history: source migration state is dirty; fix it in the source tool first")

// ErrUnsupportedSource is returned when the source holds migrations the reader
// cannot convert.
var ErrUnsupportedSource = errors.New("history: source contains unsupported migrations")

// ErrSourcePending is returned when the source tool reports that some of its
// migrations were never applied: importing them as applied would be wrong.
var ErrSourcePending = errors.New("history: source has migrations that were not applied; apply them in the source tool first")

// Raw is one migration as read by a tool-specific reader.
type Raw struct {
	Version string // the source version string
	Name    string
	Up      []byte
	Down    []byte // nil when the source has no down
	NoTx    bool   // the source marked the migration as non-transactional
	Origin  string // source file name
}

// Source is everything a reader found in the source tool's history.
type Source struct {
	Tool       string // "atlas", "goose", "golangmigrate", "bun"
	Migrations []Raw  // ascending source order
	// Applied lists the source versions the source tool recorded as applied.
	// When it is non-nil every migration must be listed.
	Applied     []string
	Dirty       bool
	Unsupported []string // file names the reader cannot convert
}

// Converted is one migration in Momo's form.
type Converted struct {
	Version         string // momo version (14-digit UTC timestamp)
	OriginalVersion string
	Name            string
	Up              []byte
	Down            []byte
	NoTx            bool
	Origin          string
}

const versionLayout = "20060102150405"

var timestampVersion = regexp.MustCompile(`^\d{14}$`)

// Convert maps the source migrations to Momo versions. A version that is a
// valid 14-digit UTC timestamp is kept; every other version maps to
// MappedBase plus its 1-based ordinal in seconds, and each mapping is recorded
// in the report as a version-mapped change. It refuses a dirty source, a
// source with unsupported files, a source with unapplied migrations, and a
// result whose versions are not strictly ascending.
func Convert(src Source) ([]Converted, *importer.Report, error) {
	if src.Dirty {
		return nil, nil, ErrSourceDirty
	}
	if len(src.Unsupported) > 0 {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnsupportedSource, strings.Join(src.Unsupported, ", "))
	}
	if src.Applied != nil {
		applied := make(map[string]bool, len(src.Applied))
		for _, v := range src.Applied {
			applied[v] = true
		}
		var missing []string
		for _, m := range src.Migrations {
			if !applied[m.Version] {
				missing = append(missing, m.Version)
			}
		}
		if len(missing) > 0 {
			return nil, nil, fmt.Errorf("%w: %s", ErrSourcePending, strings.Join(missing, ", "))
		}
	}
	base, err := time.Parse(versionLayout, MappedBase)
	if err != nil {
		return nil, nil, err
	}
	rep := &importer.Report{}
	out := make([]Converted, 0, len(src.Migrations))
	for i, m := range src.Migrations {
		c := Converted{OriginalVersion: m.Version, Name: fileName(m.Name), Up: m.Up, Down: m.Down, NoTx: m.NoTx, Origin: m.Origin}
		if isTimestamp(m.Version) {
			c.Version = m.Version
		} else {
			c.Version = base.Add(time.Duration(i+1) * time.Second).Format(versionLayout)
			rep.Add(importer.Change{
				Kind:   importer.VersionMapped,
				Source: m.Origin,
				Message: fmt.Sprintf("source version %s mapped to %s (ordinal %d after base %s)",
					m.Version, c.Version, i+1, MappedBase),
			})
		}
		if len(out) > 0 && c.Version <= out[len(out)-1].Version {
			return nil, nil, fmt.Errorf("%w: %s (version %s) does not sort after %s; Momo applies migrations in version order",
				ErrUnsupportedSource, m.Origin, c.Version, out[len(out)-1].Version)
		}
		out = append(out, c)
	}
	return out, rep, nil
}

func isTimestamp(v string) bool {
	if !timestampVersion.MatchString(v) {
		return false
	}
	_, err := time.Parse(versionLayout, v)
	return err == nil
}

var nonName = regexp.MustCompile(`[^a-z0-9]+`)

// fileName turns a source migration name into one a Momo file name accepts:
// lowercase letters, digits and underscores, starting with a letter.
func fileName(name string) string {
	n := strings.Trim(nonName.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if n == "" || n[0] < 'a' || n[0] > 'z' {
		n = "m_" + n
	}
	return strings.TrimSuffix(n, "_")
}

// Baseline applies each converted migration to the scratch database in order
// (statements split by migrate.SplitStatements; a migration that is not NoTx
// runs in one transaction, a NoTx one statement by statement in autocommit) and
// inspects the database with d after each one. It returns one SnapshotFile per
// migration: Version is the migration's version, ParentVersion the previous
// version ("" for the first). It runs nothing on any other database; scratch
// must start empty.
func Baseline(ctx context.Context, scratch *momo.DB, d migrate.Dialect, conv []Converted) ([]migrate.SnapshotFile, error) {
	eng, err := migrate.NewEngine(scratch.Driver(), fstest.MapFS{}, migrate.WithDialect(d), migrate.WithoutSum())
	if err != nil {
		return nil, err
	}
	before, err := eng.Inspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("history: inspect the scratch database: %w", err)
	}
	if len(before.Tables)+len(before.Views)+len(before.Enums)+len(before.Functions) != 0 {
		return nil, errors.New("history: the scratch database is not empty; the baseline needs a database that starts empty")
	}
	var snaps []migrate.SnapshotFile
	parent := ""
	for _, c := range conv {
		stmts, err := migrate.SplitStatements(c.Up, string(d.Dialect().Name))
		if err != nil {
			return nil, fmt.Errorf("history: %s: %w", c.Origin, err)
		}
		if err := runMigration(ctx, scratch, d, c.Origin, stmts, c.NoTx); err != nil {
			return nil, err
		}
		s, err := eng.Inspect(ctx)
		if err != nil {
			return nil, fmt.Errorf("history: inspect the scratch database after %s: %w", c.Origin, err)
		}
		snaps = append(snaps, migrate.SnapshotFile{Version: c.Version, ParentVersion: parent, Schema: s})
		parent = c.Version
	}
	return snaps, nil
}

// runMigration runs the statements of one migration on one pinned connection,
// verbatim (no placeholder scanning, so a jsonb ? operator is safe), then
// resets the session and releases the connection. Session state (SET
// search_path, SET ROLE, temporary tables) therefore lasts within a migration
// and never carries over to the next one or to the Inspect that follows, which
// takes the connection back from the pool; a one-connection pool works.
//
// Unless noTx is set the statements run between BEGIN and COMMIT, the way the
// source tools and momo migrate apply run them, so ON COMMIT DROP temporary
// tables, SET LOCAL and deferred constraints behave as they did when the
// migration was applied. A failing statement rolls the transaction back. A
// noTx migration runs each statement in autocommit.
func runMigration(ctx context.Context, scratch *momo.DB, d migrate.Dialect, origin string, stmts []string, noTx bool) (err error) {
	pinner, ok := scratch.Driver().(momo.Pinner)
	if !ok {
		return errors.New("history: the scratch database driver cannot pin a connection")
	}
	pc, err := pinner.Pin(ctx)
	if err != nil {
		return fmt.Errorf("history: pin scratch connection: %w", err)
	}
	defer func() {
		if d.Dialect().Name == dialect.Postgres {
			// DISCARD ALL drops temporary tables and restores every session setting.
			if _, rerr := pc.Exec(ctx, "DISCARD ALL", nil); rerr != nil {
				err = errors.Join(err, fmt.Errorf("history: reset the scratch session: %w", rerr))
			}
		}
		if rerr := pc.Release(); rerr != nil {
			err = errors.Join(err, rerr)
		}
	}()
	inTx := false
	if !noTx {
		if _, err := pc.Exec(ctx, "BEGIN", nil); err != nil {
			return fmt.Errorf("history: %s begin on the scratch database: %w", origin, err)
		}
		inTx = true
	}
	for i, s := range stmts {
		if _, err := pc.Exec(ctx, s, nil); err != nil {
			err = fmt.Errorf("history: %s statement %d on the scratch database: %w", origin, i+1, err)
			if inTx {
				if _, rerr := pc.Exec(ctx, "ROLLBACK", nil); rerr != nil {
					err = errors.Join(err, fmt.Errorf("history: roll back %s on the scratch database: %w", origin, rerr))
				}
			}
			return err
		}
	}
	if inTx {
		if _, err := pc.Exec(ctx, "COMMIT", nil); err != nil {
			return fmt.Errorf("history: %s commit on the scratch database: %w", origin, err)
		}
	}
	return nil
}

var (
	versionRe = regexp.MustCompile(`^\d{14}$`)
	nameRe    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// WriteImport writes the converted migrations into outDir: <version>_<name>.up.sql
// (first line "-- momo:imported", so migrate validate does not take it for a
// custom migration that must leave the schema alone and rebase never
// regenerates it, then "-- momo:no-transaction"
// when NoTx), the .down.sql file when the
// source had a down, <version>_<name>.snapshot.json for each snapshot, and
// momo.sum for the directory. Everything is validated first and written to a
// temporary directory beside outDir, which is then renamed into place, so a
// failure leaves nothing behind and a retry works. outDir must not exist or
// must be empty; existing files are never overwritten. Parent directories
// created with MkdirAll are left in place when the import fails.
func WriteImport(outDir string, conv []Converted, snaps []migrate.SnapshotFile) error {
	byVersion := map[string]Converted{}
	for _, c := range conv {
		if !versionRe.MatchString(c.Version) || !nameRe.MatchString(c.Name) {
			return fmt.Errorf("history: %s: version %q or name %q is not usable in a file name", c.Origin, c.Version, c.Name)
		}
		if _, dup := byVersion[c.Version]; dup {
			return fmt.Errorf("history: version %s appears twice", c.Version)
		}
		byVersion[c.Version] = c
	}
	seenSnap := map[string]bool{}
	for _, sn := range snaps {
		if _, ok := byVersion[sn.Version]; !ok {
			return fmt.Errorf("history: snapshot for version %s has no converted migration", sn.Version)
		}
		if seenSnap[sn.Version] {
			return fmt.Errorf("history: two snapshots for version %s", sn.Version)
		}
		seenSnap[sn.Version] = true
	}
	outDir = filepath.Clean(outDir)
	if ents, err := os.ReadDir(outDir); err == nil && len(ents) > 0 {
		return fmt.Errorf("history: %s is not empty; the import never overwrites", outDir)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("history: %w", err)
	}
	parent := filepath.Dir(outDir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return fmt.Errorf("history: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".momo-import-*")
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()
	create := func(name string, data []byte) error {
		f, err := os.OpenFile(filepath.Join(stage, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- name is built from a validated version and name, never from path input
		if err != nil {
			return fmt.Errorf("history: %w", err)
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return fmt.Errorf("history: writing %s: %w", name, err)
		}
		return f.Close()
	}
	for _, c := range conv {
		base := c.Version + "_" + c.Name
		header := "-- momo:imported\n"
		if c.NoTx {
			header += "-- momo:no-transaction\n"
		}
		up := append([]byte(header), c.Up...)
		if err := create(base+".up.sql", up); err != nil {
			return err
		}
		if c.Down != nil {
			if err := create(base+".down.sql", c.Down); err != nil {
				return err
			}
		}
	}
	for _, sn := range snaps {
		c := byVersion[sn.Version]
		var buf bytes.Buffer
		if err := migrate.WriteSnapshot(&buf, sn); err != nil {
			return err
		}
		if err := create(c.Version+"_"+c.Name+".snapshot.json", buf.Bytes()); err != nil {
			return err
		}
	}
	sum, err := migrate.ComputeSum(os.DirFS(stage))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := sum.Write(&buf); err != nil {
		return err
	}
	if err := create(migrate.SumFileName, buf.Bytes()); err != nil {
		return err
	}
	if err := os.Chmod(stage, 0o750); err != nil { // #nosec G302 -- a directory; 0750 like the other generated directories
		return fmt.Errorf("history: %w", err)
	}
	// Windows cannot rename onto an existing directory, so remove the empty one
	// first (os.Remove refuses a directory that is not empty).
	if err := os.Remove(outDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("history: %w", err)
	}
	if err := os.Rename(stage, outDir); err != nil {
		// Best effort: put back the empty directory removed above.
		_ = os.Mkdir(outDir, 0o750)
		return fmt.Errorf("history: %w", err)
	}
	committed = true
	return nil
}

// HistoryRows returns the momo_migrations rows that record conv as applied:
// Name is OriginalVersion+"_"+Name when the version was mapped (so the source
// version is kept), else Name; Checksum comes from sum for the migration's up
// file; the status is applied with zero statements, ApplyID "import:<tool>"
// and Kind "sql". It fails when sum is nil or lacks an entry for a migration:
// a row without the checksum would make the first migrate up report a
// checksum mismatch.
func HistoryRows(conv []Converted, sum *migrate.Sum, tool string, now time.Time) ([]migrate.HistoryRow, error) {
	if sum == nil {
		return nil, errors.New("history: HistoryRows needs the momo.sum of the written directory")
	}
	rows := make([]migrate.HistoryRow, 0, len(conv))
	for _, c := range conv {
		name := c.Name
		if c.OriginalVersion != c.Version {
			name = c.OriginalVersion + "_" + c.Name
		}
		file := c.Version + "_" + c.Name + ".up.sql"
		checksum := sum.Files[file]
		if checksum == "" {
			return nil, fmt.Errorf("history: momo.sum has no entry for %s", file)
		}
		rows = append(rows, migrate.HistoryRow{
			Version: c.Version, Name: name, Checksum: checksum, Status: migrate.StatusApplied,
			ApplyID: "import:" + tool, Kind: "sql", AppliedAt: now,
		})
	}
	return rows, nil
}
