// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrHandwritten is returned when an output path holds a file that does not
// carry the generated header.
var ErrHandwritten = errors.New("refusing to overwrite a handwritten file")

// ErrCollision is returned when two different outputs map to one path.
var ErrCollision = errors.New("two different outputs for one path")

// WriterOptions configures a Writer.
type WriterOptions struct {
	// ModuleRoot is the absolute directory every output must live under.
	ModuleRoot string
	// Check makes Commit report differences instead of writing.
	Check bool
}

// Writer buffers generated files and commits them atomically and
// deterministically. It is not safe for concurrent use.
type Writer struct {
	options WriterOptions
	files   map[string][]byte
}

// NewWriter returns a Writer for opts.
func NewWriter(opts WriterOptions) *Writer {
	opts.ModuleRoot = filepath.Clean(opts.ModuleRoot)
	return &Writer{options: opts, files: map[string][]byte{}}
}

// resolveExisting resolves symlinks in the nearest existing ancestor of path
// and re-appends the not yet existing remainder.
func resolveExisting(path string) (string, error) {
	rest := ""
	cur := filepath.Clean(path)
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(resolved, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// contained reports whether path, with symlinks resolved, lies under the
// resolved module root.
func (w *Writer) contained(path string) (bool, error) {
	root, err := resolveExisting(w.options.ModuleRoot)
	if err != nil {
		return false, err
	}
	target, err := resolveExisting(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

// File buffers src (formatted with go/format) as the content of path.
func (w *Writer) File(path string, src []byte) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("gen: output path %q is not absolute", path)
	}
	path = filepath.Clean(path)
	ok, err := w.contained(path)
	if err != nil {
		return fmt.Errorf("gen: resolving %s: %w", path, err)
	}
	if !ok {
		return fmt.Errorf("gen: output path %q is outside the module root %q", path, w.options.ModuleRoot)
	}
	formatted, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("gen: generator bug: formatting %s: %w", path, err)
	}
	hand, err := isHandwritten(path)
	if err != nil {
		return err
	}
	if hand {
		return fmt.Errorf("gen: %s: %w", path, ErrHandwritten)
	}
	if !bytes.HasPrefix(formatted, []byte(HeaderPrefix)) {
		return fmt.Errorf("gen: generator bug: output for %s lacks the %q header", path, strings.TrimSpace(HeaderPrefix))
	}
	if prev, ok := w.files[path]; ok {
		if !bytes.Equal(prev, formatted) {
			return fmt.Errorf("gen: %s: %w", path, ErrCollision)
		}
		return nil
	}
	w.files[path] = formatted
	return nil
}

// isHandwritten reports whether path exists and lacks the generated header.
func isHandwritten(path string) (bool, error) {
	has, exists, err := hasHeader(path)
	if err != nil {
		return false, err
	}
	return exists && !has, nil
}

// hasHeader reads only the first line of path.
func hasHeader(path string) (header, exists bool, err error) {
	f, err := os.Open(path) // #nosec G304 -- callers pass only paths that File() cleaned and confined to the module root (symlinks resolved)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("gen: %w", err)
	}
	defer func() { _ = f.Close() }()
	line, err := bufio.NewReader(f).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, true, fmt.Errorf("gen: reading %s: %w", path, err)
	}
	return isHeaderLine(line), true, nil
}

// commentLeaders are the line-comment leaders of the generated text formats:
// Go, protobuf and DOT; Mermaid; and GraphQL schema and YAML.
var commentLeaders = []string{"// ", "%% ", "# "}

// isHeaderLine reports whether line is the generated-file header in any of
// the comment styles Text accepts.
func isHeaderLine(line string) bool {
	for _, lead := range commentLeaders {
		if strings.HasPrefix(line, lead+strings.TrimPrefix(HeaderPrefix, "// ")) {
			return true
		}
	}
	return false
}

// Text buffers data, which is not Go source, as the content of path: a
// Mermaid, DOT, protobuf, GraphQL schema or YAML file. It follows the refusal
// rules of File (absolute path inside the module root, no overwriting of a
// handwritten file, no two different outputs for one path) but never
// formats. The caller puts the generated header, in the comment style of the
// format, on the first line of data; the writer refuses data without it.
func (w *Writer) Text(path string, data []byte) error {
	path, err := w.confine(path)
	if err != nil {
		return err
	}
	first, _, _ := strings.Cut(string(data), "\n")
	if !isHeaderLine(strings.TrimRight(first, "\r")) {
		return fmt.Errorf("gen: generator bug: output for %s lacks the %q header", path, strings.TrimSpace(HeaderPrefix))
	}
	hand, err := isHandwritten(path)
	if err != nil {
		return err
	}
	if hand {
		return fmt.Errorf("gen: %s: %w", path, ErrHandwritten)
	}
	return w.put(path, append([]byte(nil), data...))
}

// Scaffold buffers data as the content of path only when path does not exist
// yet. A scaffold is a starting point the user owns, not a generated file: it
// carries no header and is never overwritten, so a later run leaves the
// user's edits alone. Go source is not formatted.
func (w *Writer) Scaffold(path string, data []byte) error {
	path, err := w.confine(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("gen: %w", err)
	}
	return w.put(path, append([]byte(nil), data...))
}

// confine cleans path and checks that it is absolute and inside the module
// root once symlinks are resolved.
func (w *Writer) confine(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("gen: output path %q is not absolute", path)
	}
	path = filepath.Clean(path)
	ok, err := w.contained(path)
	if err != nil {
		return "", fmt.Errorf("gen: resolving %s: %w", path, err)
	}
	if !ok {
		return "", fmt.Errorf("gen: output path %q is outside the module root %q", path, w.options.ModuleRoot)
	}
	return path, nil
}

// put buffers data for path, refusing a different earlier output.
func (w *Writer) put(path string, data []byte) error {
	if prev, ok := w.files[path]; ok {
		if !bytes.Equal(prev, data) {
			return fmt.Errorf("gen: %s: %w", path, ErrCollision)
		}
		return nil
	}
	w.files[path] = data
	return nil
}

// Commit writes every buffered file (or, in check mode, reports the ones
// that differ) and returns the changed paths in sorted order.
func (w *Writer) Commit() (changed []string, err error) {
	paths := make([]string, 0, len(w.files))
	for p := range w.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		// Generated output is LF; a CRLF copy (core.autocrlf checkout) is up to date.
		if cur, rerr := os.ReadFile(p); rerr == nil && bytes.Equal(bytes.ReplaceAll(cur, []byte("\r\n"), []byte("\n")), w.files[p]) { // #nosec G304 -- p is a key of w.files, each confined to the module root by File()
			continue
		}
		if hand, herr := isHandwritten(p); herr != nil {
			return changed, herr
		} else if hand {
			return changed, fmt.Errorf("gen: %s: %w", p, ErrHandwritten)
		}
		if !w.options.Check {
			if err := writeAtomic(p, w.files[p]); err != nil {
				return changed, err
			}
		}
		changed = append(changed, p)
	}
	return changed, nil
}

// writeAtomic writes data to a temp file beside path, then renames it.
func writeAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- generated source directories are committed, not secret; 0755 like any checked-in directory
		return fmt.Errorf("gen: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".momo-*.tmp")
	if err != nil {
		return fmt.Errorf("gen: %w", err)
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(name)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("gen: writing %s: %w", path, err)
	}
	if err = tmp.Chmod(0o644); err != nil {
		return fmt.Errorf("gen: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("gen: %w", err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("gen: %w", err)
	}
	return nil
}

// Orphans returns generated files under dirs that this run did not produce.
func (w *Writer) Orphans(dirs []string) ([]string, error) {
	var out []string
	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if p == dir && errors.Is(err, fs.ErrNotExist) {
					return fs.SkipDir
				}
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") {
				return nil
			}
			p = filepath.Clean(p)
			if _, ok := w.files[p]; ok {
				return nil
			}
			has, _, herr := hasHeader(p)
			if herr != nil {
				return herr
			}
			if has {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("gen: %w", err)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Output is one buffered generated file.
type Output struct {
	Path string
	Data []byte
}

// Outputs returns the buffered files sorted by Path. It is available before
// Commit and returns copies, so callers cannot alter what Commit writes.
func (w *Writer) Outputs() []Output {
	out := make([]Output, 0, len(w.files))
	for p, data := range w.files {
		out = append(out, Output{Path: p, Data: append([]byte(nil), data...)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
