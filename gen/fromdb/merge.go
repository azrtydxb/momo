// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The kinds of Action.
const (
	ActionCreate    = "create"    // the file does not exist yet
	ActionUpdate    = "update"    // the file was not edited by hand and has changed
	ActionUnchanged = "unchanged" // the file is current
	ActionConflict  = "conflict"  // the file was edited by hand and the database changed it
	ActionOrphan    = "orphan"    // the file models a table that is gone or skipped
)

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
// overwrite is set.
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
		if e.IsDir() || want[e.Name()] || !strings.HasSuffix(e.Name(), "_model.go") {
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
	edited, generated := Edited(existing)
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

// Apply carries out the actions. A conflict writes the regenerated version
// next to the file with the .new suffix and leaves the file alone. It returns
// the paths it wrote.
func Apply(dir string, actions []Action) ([]string, error) {
	var written []string
	for _, a := range actions {
		var name string
		switch a.Kind {
		case ActionCreate, ActionUpdate:
			name = a.File
		case ActionConflict:
			name = a.File + newSuffix
		default:
			continue
		}
		p := filepath.Join(dir, name)
		if err := writeAtomic(p, a.Content); err != nil {
			return written, err
		}
		written = append(written, p)
	}
	return written, nil
}

// writeAtomic writes through a temporary file in the same directory, so a
// crash never leaves a half-written model.
func writeAtomic(path string, content []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- model source directories are committed, not secret
		return fmt.Errorf("fromdb: create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".momo-*.tmp")
	if err != nil {
		return fmt.Errorf("fromdb: write %s: %w", path, err)
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if _, err = tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fromdb: write %s: %w", path, err)
	}
	if err = tmp.Chmod(0o644); err != nil { // #nosec G302 -- model source files are committed and read by the team
		_ = tmp.Close()
		return fmt.Errorf("fromdb: write %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("fromdb: write %s: %w", path, err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("fromdb: write %s: %w", path, err)
	}
	return nil
}
