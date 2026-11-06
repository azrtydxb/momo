// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// SumFormat is the momo.sum format version this momo writes and reads up to.
const SumFormat = 1

// SumFileName is the checksum file inside a migration directory.
const SumFileName = "momo.sum"

// ErrNewerFormat is returned when a persisted migration artifact (momo.sum or
// a snapshot) was written by a newer momo than this one.
var ErrNewerFormat = schema.ErrNewerFormat

// Sum is the parsed content of momo.sum.
type Sum struct {
	DirHash string            // "h1:" + base64(sha256(manifest))
	Files   map[string]string // file name -> hex sha256
}

var (
	sumHeader = regexp.MustCompile(`^# momo\.sum format=(\d+)$`)
	hexSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ComputeSum hashes every file in fsys, including nested directories, except
// the root momo.sum and dotfiles or dot directories (.DS_Store, .git and the
// .momo-*.tmp files of an interrupted write), which are skipped everywhere in
// the walk. Entries are slash-separated paths relative to fsys.
func ComputeSum(fsys fs.FS) (*Sum, error) {
	files := map[string]string{}
	err := walkFiles(fsys, func(name string) error {
		if strings.ContainsAny(name, "\r\n") || name != strings.TrimSpace(name) {
			return fmt.Errorf("migrate: file name %q cannot be recorded in %s", name, SumFileName)
		}
		h, err := hashFile(fsys, name)
		if err != nil {
			return err
		}
		files[name] = h
		return nil
	})
	if err != nil {
		if strings.HasPrefix(err.Error(), "migrate: ") || strings.HasPrefix(err.Error(), walkErrPrefix) {
			return nil, err
		}
		return nil, fmt.Errorf("migrate: read migration directory: %w", err)
	}
	return &Sum{DirHash: dirHash(files), Files: files}, nil
}

func hashFile(fsys fs.FS, name string) (string, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", fmt.Errorf("migrate: read %s: %w", name, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func dirHash(files map[string]string) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(files)) {
		b.WriteString(name + "\n" + files[name] + "\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "h1:" + base64.StdEncoding.EncodeToString(sum[:])
}

// Write writes s in momo.sum format with the file lines in name order.
func (s *Sum) Write(w io.Writer) error {
	var b strings.Builder
	b.WriteString("# momo.sum format=" + strconv.Itoa(SumFormat) + "\n")
	b.WriteString(dirHash(s.Files) + "\n")
	for _, name := range slices.Sorted(maps.Keys(s.Files)) {
		b.WriteString(s.Files[name] + "  " + name + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// ParseSum reads a momo.sum. A format above SumFormat returns ErrNewerFormat.
// A directory hash that does not match the listed files returns
// ErrChecksumMismatch: the file was edited by hand.
func ParseSum(r io.Reader) (*Sum, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("migrate: read %s: %w", SumFileName, err)
	}
	if len(lines) > 0 {
		lines[0] = strings.TrimPrefix(lines[0], string(utf8BOM))
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("migrate: %s line 1: missing header %q", SumFileName, "# momo.sum format=N")
	}
	m := sumHeader.FindStringSubmatch(lines[0])
	if m == nil {
		return nil, fmt.Errorf("migrate: %s line 1: want header %q, got %q", SumFileName, "# momo.sum format=N", lines[0])
	}
	got, err := strconv.Atoi(m[1])
	if err != nil || got < 1 {
		return nil, fmt.Errorf("migrate: %s line 1: invalid format %q", SumFileName, m[1])
	}
	if got > SumFormat {
		return nil, fmt.Errorf("%w (momo.sum format %d, this momo reads up to %d)", ErrNewerFormat, got, SumFormat)
	}
	if len(lines) < 2 || !strings.HasPrefix(lines[1], "h1:") {
		return nil, fmt.Errorf("migrate: %s line 2: want an h1: directory hash", SumFileName)
	}
	s := &Sum{DirHash: lines[1], Files: map[string]string{}}
	for i, line := range lines[2:] {
		n := i + 3
		sum, name, ok := strings.Cut(line, "  ")
		if !ok || !hexSHA256.MatchString(sum) || name == "" || name != strings.TrimSpace(name) {
			return nil, fmt.Errorf("migrate: %s line %d: want %q", SumFileName, n, "<sha256 hex>  <file name>")
		}
		if _, dup := s.Files[name]; dup {
			return nil, fmt.Errorf("migrate: %s line %d: duplicate entry for %s", SumFileName, n, name)
		}
		s.Files[name] = sum
	}
	if want := dirHash(s.Files); want != s.DirHash {
		return nil, fmt.Errorf("%w: %s directory hash does not match its file lines; the file was edited by hand", ErrChecksumMismatch, SumFileName)
	}
	return s, nil
}

// VerifySum returns the sorted names of files whose content differs from
// want, plus names present on only one side. An unreadable file counts as
// differing. A clean directory returns nil.
func VerifySum(fsys fs.FS, want *Sum) []string {
	bad := map[string]bool{}
	seen := map[string]bool{}
	// A directory that cannot be walked yields only what was read before the error.
	_ = walkFiles(fsys, func(name string) error {
		seen[name] = true
		h, err := hashFile(fsys, name)
		if wantHash, ok := want.Files[name]; err != nil || !ok || h != wantHash {
			bad[name] = true
		}
		return nil
	})
	for name := range want.Files {
		if !seen[name] {
			bad[name] = true
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(bad))
}

// AppendSum returns a copy of existing with entries for the added files. It
// never changes an existing entry; an added name that is already recorded, or
// that is not in fsys, is an error naming it.
func AppendSum(existing *Sum, fsys fs.FS, added []string) (*Sum, error) {
	files := map[string]string{}
	if existing != nil {
		maps.Copy(files, existing.Files)
	}
	for _, name := range added {
		if _, ok := files[name]; ok {
			return nil, errors.New("migrate: " + name + " is already recorded in " + SumFileName + "; recorded checksums are never rewritten")
		}
		if strings.ContainsAny(name, "\r\n") || name != strings.TrimSpace(name) || name == SumFileName {
			return nil, fmt.Errorf("migrate: file name %q cannot be recorded in %s", name, SumFileName)
		}
		h, err := hashFile(fsys, name)
		if err != nil {
			return nil, err
		}
		files[name] = h
	}
	return &Sum{DirHash: dirHash(files), Files: files}, nil
}
