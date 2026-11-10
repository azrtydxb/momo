// SPDX-License-Identifier: Apache-2.0

// Package atlas reads an Atlas migration directory (<version>_<name>.sql files
// and atlas.sum) and the atlas_schema_revisions table into a history.Source.
//
// The atlas.sum check follows ariga.io/atlas/sql/migrate (HashFile, NewHashFile
// and HashFile.Sum in dir.go, read at v0.36.2): the hash of each file is the
// running SHA-256 over every file name and body up to and including that
// file, and the first line of atlas.sum is the SHA-256 of all names and file
// hashes. Atlas itself is not a dependency.
package atlas

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/importer/history"
)

// SumFile is the name of the Atlas directory checksum file.
const SumFile = "atlas.sum"

// Revision is one row of atlas_schema_revisions as the caller read it.
type Revision struct {
	Version        string
	Applied, Total int
	Error          string
}

var (
	fileRe      = regexp.MustCompile(`^(\d+)_(.+)\.sql$`)
	directiveRe = regexp.MustCompile(`^([ -~]*)atlas:(\w+)(?: +([ -~]*))*`)
)

// Read returns a history.Source for an Atlas directory: files
// <version>_<name>.sql in ascending version order; Up is the file body and
// Down is nil. atlas.sum is verified first; a mismatch returns an error naming
// the file. revisions are the rows of atlas_schema_revisions as the caller
// read them: a version is Applied when Applied == Total and Error is empty;
// Dirty is true when any row has a non-empty Error or Applied < Total. A file
// with "-- atlas:txmode none" on its first line is read as NoTx.
func Read(dir fs.FS, revisions []Revision) (history.Source, error) {
	src := history.Source{Tool: "atlas", Applied: []string{}}
	ents, err := fs.ReadDir(dir, ".")
	if err != nil {
		return src, fmt.Errorf("atlas: %w", err)
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	bodies := make(map[string][]byte, len(names))
	for _, n := range names {
		b, err := fs.ReadFile(dir, n)
		if err != nil {
			return src, fmt.Errorf("atlas: %w", err)
		}
		bodies[n] = b
	}
	if err := verifySum(dir, names, bodies); err != nil {
		return src, err
	}
	for _, n := range names {
		m := fileRe.FindStringSubmatch(n)
		if m == nil {
			src.Unsupported = append(src.Unsupported, n)
			continue
		}
		src.Migrations = append(src.Migrations, history.Raw{
			Version: m[1], Name: m[2], Up: bodies[n], NoTx: noTx(bodies[n]), Origin: n,
		})
	}
	// Ascending version order, compared as numbers (same-length timestamps
	// sort the same as strings; a shorter version sorts first).
	sort.SliceStable(src.Migrations, func(i, j int) bool {
		a, b := src.Migrations[i].Version, src.Migrations[j].Version
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return a < b
	})
	done := map[string]bool{}
	for _, r := range revisions {
		if r.Error != "" || r.Applied < r.Total {
			src.Dirty = true
			continue
		}
		done[r.Version] = true
	}
	for _, m := range src.Migrations {
		if done[m.Version] {
			src.Applied = append(src.Applied, m.Version)
		}
	}
	return src, nil
}

// noTx reports a first-line "atlas:txmode none" directive.
func noTx(body []byte) bool {
	m := directiveRe.FindStringSubmatch(string(body))
	return len(m) == 4 && m[2] == "txmode" && strings.TrimSpace(m[3]) == "none"
}

// sumIgnored reports a first-line "atlas:sum ignore" directive: Atlas leaves
// such a file out of atlas.sum.
func sumIgnored(body []byte) bool {
	m := directiveRe.FindStringSubmatch(string(body))
	return len(m) == 4 && m[2] == "sum" && m[3] == "ignore"
}

type fileHash struct{ name, hash string }

// computeSum is NewHashFile and HashFile.Sum of ariga.io/atlas.
func computeSum(names []string, bodies map[string][]byte) (total string, files []fileHash) {
	h := sha256.New()
	for _, n := range names {
		_, _ = h.Write([]byte(n))
		if sumIgnored(bodies[n]) {
			continue
		}
		_, _ = h.Write(bodies[n])
		files = append(files, fileHash{n, base64.StdEncoding.EncodeToString(h.Sum(nil))})
	}
	t := sha256.New()
	for _, f := range files {
		_, _ = t.Write([]byte(f.name))
		_, _ = t.Write([]byte(f.hash))
	}
	return base64.StdEncoding.EncodeToString(t.Sum(nil)), files
}

func verifySum(dir fs.FS, names []string, bodies map[string][]byte) error {
	raw, err := fs.ReadFile(dir, SumFile)
	if err != nil {
		return fmt.Errorf("atlas: %s is required to verify the directory: %w", SumFile, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	if !sc.Scan() || !strings.HasPrefix(sc.Text(), "h1:") {
		return fmt.Errorf("atlas: %s: the first line must be h1:<checksum>", SumFile)
	}
	wantTotal := strings.TrimPrefix(sc.Text(), "h1:")
	var want []fileHash
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		i := strings.LastIndex(line, " h1:")
		if i < 0 {
			return fmt.Errorf("atlas: %s: malformed line %q", SumFile, line)
		}
		want = append(want, fileHash{line[:i], line[i+len(" h1:"):]})
	}
	total, got := computeSum(names, bodies)
	have := make(map[string]string, len(want))
	for _, w := range want {
		have[w.name] = w.hash
	}
	for _, g := range got {
		w, ok := have[g.name]
		if !ok {
			return fmt.Errorf("atlas: %s is not listed in %s", g.name, SumFile)
		}
		if w != g.hash {
			return fmt.Errorf("atlas: %s does not match %s (the file was edited after the checksum was written)", g.name, SumFile)
		}
		delete(have, g.name)
	}
	for _, w := range want {
		if _, left := have[w.name]; left {
			return fmt.Errorf("atlas: %s is listed in %s but missing from the directory", w.name, SumFile)
		}
	}
	if total != wantTotal {
		return fmt.Errorf("atlas: the directory checksum on the first line of %s does not match its files", SumFile)
	}
	return nil
}
