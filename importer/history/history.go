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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/importer"
	"github.com/azrtydxb/momo/migrate"
)

// MappedBase is the version a source version that is not a timestamp is
// mapped after: the n-th such migration (1-based, in source order) gets
// MappedBase plus n seconds.
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
// (statements split by migrate.SplitStatements, each autocommitted) and
// inspects the database with d after each one. It returns one SnapshotFile per
// migration: Version is the migration's version, ParentVersion the previous
// version ("" for the first). It runs nothing on any other database; scratch
// must start empty.
func Baseline(ctx context.Context, scratch *momo.DB, d migrate.Dialect, conv []Converted) ([]migrate.SnapshotFile, error) {
	pinner, ok := scratch.Driver().(momo.Pinner)
	if !ok {
		return nil, errors.New("history: the scratch database driver cannot pin a connection")
	}
	pc, err := pinner.Pin(ctx)
	if err != nil {
		return nil, fmt.Errorf("history: pin scratch connection: %w", err)
	}
	defer func() { _ = pc.Release() }()
	eng, err := migrate.NewEngine(scratch.Driver(), fstest.MapFS{}, migrate.WithDialect(d), migrate.WithoutSum())
	if err != nil {
		return nil, err
	}
	var snaps []migrate.SnapshotFile
	parent := ""
	for _, c := range conv {
		stmts, err := migrate.SplitStatements(c.Up, string(d.Dialect().Name))
		if err != nil {
			return nil, fmt.Errorf("history: %s: %w", c.Origin, err)
		}
		for i, s := range stmts {
			if _, err := pc.Exec(ctx, s, nil); err != nil {
				return nil, fmt.Errorf("history: %s statement %d on the scratch database: %w", c.Origin, i+1, err)
			}
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

// WriteImport writes the converted migrations into outDir: <version>_<name>.up.sql
// (first line "-- momo:no-transaction" when NoTx), the .down.sql file when the
// source had a down, <version>_<name>.snapshot.json for each snapshot, and
// momo.sum for the directory. It never overwrites: files are created
// exclusively, and an existing file is an error.
func WriteImport(outDir string, conv []Converted, snaps []migrate.SnapshotFile) error {
	byVersion := map[string]Converted{}
	for _, c := range conv {
		byVersion[c.Version] = c
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("history: %w", err)
	}
	create := func(name string, data []byte) error {
		f, err := os.OpenFile(filepath.Join(outDir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- name is built from a validated version and a sanitized name, never from path input
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
		up := c.Up
		if c.NoTx {
			up = append([]byte("-- momo:no-transaction\n"), up...)
		}
		if err := create(base+".up.sql", up); err != nil {
			return err
		}
		if c.Down != nil {
			if err := create(base+".down.sql", c.Down); err != nil {
				return err
			}
		}
	}
	for _, s := range snaps {
		c, ok := byVersion[s.Version]
		if !ok {
			return fmt.Errorf("history: snapshot for version %s has no converted migration", s.Version)
		}
		var buf bytes.Buffer
		if err := migrate.WriteSnapshot(&buf, s); err != nil {
			return err
		}
		if err := create(c.Version+"_"+c.Name+".snapshot.json", buf.Bytes()); err != nil {
			return err
		}
	}
	sum, err := migrate.ComputeSum(os.DirFS(outDir))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := sum.Write(&buf); err != nil {
		return err
	}
	return create(migrate.SumFileName, buf.Bytes())
}

// HistoryRows returns the momo_migrations rows that record conv as applied:
// Name is OriginalVersion+"_"+Name when the version was mapped (so the source
// version is kept), else Name; Checksum comes from sum for the migration's up
// file (empty when sum is nil); the status is applied with zero statements,
// ApplyID "import:<tool>" and Kind "sql".
func HistoryRows(conv []Converted, sum *migrate.Sum, tool string, now time.Time) []migrate.HistoryRow {
	rows := make([]migrate.HistoryRow, 0, len(conv))
	for _, c := range conv {
		name := c.Name
		if c.OriginalVersion != c.Version {
			name = c.OriginalVersion + "_" + c.Name
		}
		checksum := ""
		if sum != nil {
			checksum = sum.Files[c.Version+"_"+c.Name+".up.sql"]
		}
		rows = append(rows, migrate.HistoryRow{
			Version: c.Version, Name: name, Checksum: checksum, Status: migrate.StatusApplied,
			ApplyID: "import:" + tool, Kind: "sql", AppliedAt: now,
		})
	}
	return rows
}
