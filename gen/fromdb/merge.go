// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// The kinds of Action.
const (
	ActionCreate    = "create"    // the file does not exist yet
	ActionUpdate    = "update"    // the file was not edited by hand and has changed
	ActionUnchanged = "unchanged" // the file is current
	ActionConflict  = "conflict"  // the file was edited by hand and the database changed it
	ActionOrphan    = "orphan"    // the file models a table that is gone or skipped
	ActionHeader    = "header"    // the body is current and only the header hash is stale
)

// modelFile matches the names fileName gives, with or without a counter.
var modelFile = regexp.MustCompile(`_model[0-9]*\.go$`)

// newSuffix is added to the name of a file that holds the regenerated version
// of a conflicting file.
const newSuffix = ".new"

// Action is what a run does with one file.
type Action struct {
	Kind    string
	File    string // file name in the output directory
	Table   string
	Detail  string
	Content []byte // the content to write; nil for unchanged and orphan
}

// Plan compares the generated files with the output directory. It reads the
// directory and writes nothing. A file that was edited by hand, or that this
// command did not write, is never replaced: it becomes a conflict, unless
// overwrite is set. A hand-edited file whose model the database did not change
// stays unchanged even with overwrite, so overwrite never discards an edit that
// the database gave no reason to replace.
func Plan(dir string, files []File, overwrite bool) ([]Action, error) {
	var out []Action
	want := map[string]bool{}
	for _, f := range files {
		want[f.Name] = true
		a, err := planFile(dir, f, overwrite)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("fromdb: read %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || want[e.Name()] || !modelFile.MatchString(e.Name()) {
			continue
		}
		b, err := readFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if _, generated := Edited(b); generated {
			out = append(out, Action{Kind: ActionOrphan, File: e.Name(), Detail: "no table of the database or of this run models it any more; delete it if the table is gone"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
}

func planFile(dir string, f File, overwrite bool) (Action, error) {
	a := Action{File: f.Name, Table: f.Table, Content: f.Content}
	existing, err := readFile(filepath.Join(dir, f.Name))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		a.Kind = ActionCreate
		return a, nil
	case err != nil:
		return Action{}, err
	}
	if bytes.Equal(existing, f.Content) {
		a.Kind, a.Content = ActionUnchanged, nil
		return a, nil
	}
	if refreshed, ok := refreshHeader(existing, f.Content); ok {
		// The body is the regenerated body, so only the hash in the header
		// line is out of date, for example after a hand edit that the database
		// now carries too. --check treats the file as current; a plain run
		// rewrites line 1 and nothing else.
		a.Kind, a.Content = ActionHeader, refreshed
		return a, nil
	}
	edited, generated := Edited(existing)
	if generated && edited {
		// Edited by hand, but the database did not change the model since it
		// was written: the header hash is still the hash of the new body.
		if _, newHash, ok := SplitHeader(f.Content); ok {
			if _, oldHash, ok := SplitHeader(existing); ok && newHash == oldHash {
				a.Kind, a.Content = ActionUnchanged, nil
				return a, nil
			}
		}
	}
	switch {
	case overwrite:
		a.Kind = ActionUpdate
	case !generated:
		a.Kind = ActionConflict
		a.Detail = "the file exists and was not written by --from-db"
	case edited:
		a.Kind = ActionConflict
		a.Detail = "the file has hand edits and the database changed the model"
	default:
		a.Kind = ActionUpdate
	}
	return a, nil
}

// refreshHeader reports whether existing is a file written by WithHeader whose
// body equals the body of regenerated although the hash in its header differs,
// and returns existing with only its first line replaced by the header of
// regenerated. Line endings are normalised to LF for the comparison, as the
// plain generator does for a checkout with CRLF; the rest of the file, its
// line endings included, is kept byte for byte.
func refreshHeader(existing, regenerated []byte) ([]byte, bool) {
	newBody, newHash, ok := SplitHeader(regenerated)
	if !ok {
		return nil, false
	}
	oldBody, oldHash, ok := SplitHeader(bytes.ReplaceAll(existing, []byte("\r\n"), []byte("\n")))
	if !ok || oldHash == newHash || !bytes.Equal(oldBody, newBody) {
		return nil, false
	}
	i := bytes.IndexByte(existing, '\n')
	if i < 0 {
		return nil, false
	}
	line := headerPrefix + newHash
	if i > 0 && existing[i-1] == '\r' {
		line += "\r"
	}
	return append([]byte(line), existing[i:]...), true
}

func readFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- path is in the model directory the user named
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("fromdb: read %s: %w", path, err)
	}
	return b, nil
}

// Apply carries out the actions through write, which stores one file. A
// header action writes the file with its first line replaced. A conflict writes the regenerated version next to the file with the .new
// suffix and leaves the file alone. It returns the paths it wrote. The caller
// supplies write so that this package never touches the filesystem itself.
func Apply(dir string, actions []Action, write func(path string, content []byte) error) ([]string, error) {
	var written []string
	for _, a := range actions {
		var name string
		switch a.Kind {
		case ActionCreate, ActionUpdate, ActionHeader:
			name = a.File
		case ActionConflict:
			name = a.File + newSuffix
		default:
			continue
		}
		p := filepath.Join(dir, name)
		if err := write(p, a.Content); err != nil {
			return written, err
		}
		written = append(written, p)
	}
	return written, nil
}
