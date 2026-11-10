// SPDX-License-Identifier: Apache-2.0

// Package goose reads a goose migration directory and its version table into
// a history.Source. The version table layout (id, version_id, is_applied,
// tstamp) and the annotation grammar (-- +goose Up, Down, StatementBegin,
// StatementEnd, NO TRANSACTION) follow github.com/pressly/goose/v3; goose
// itself is not a dependency. Go migrations (.go files) are listed as
// unsupported.
package goose

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/importer/history"
	"github.com/azrtydxb/momo/importer/history/internal/histutil"
)

var fileRe = regexp.MustCompile(`^(\d+)_(.+)\.sql$`)

// splitGoose splits a goose file body into up and down. The goose directive
// lines are dropped and every other line is kept verbatim, so a statement
// block between StatementBegin and StatementEnd stays whole (the splitter that
// runs later reads dollar-quoted bodies as one statement). down is nil when
// the file has no Down section or it holds nothing but blank lines.
func splitGoose(body []byte) (up, down []byte, err error) {
	up, down, _, err = parseFile(body)
	return up, down, err
}

// parseFile is splitGoose plus the -- +goose NO TRANSACTION annotation.
func parseFile(body []byte) (up, down []byte, noTx bool, err error) {
	var upBuf, downBuf bytes.Buffer
	section := ""
	inBlock := false
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if cmd, ok := strings.CutPrefix(strings.TrimSpace(line), "--"); ok {
			if arg, ok := strings.CutPrefix(strings.TrimSpace(cmd), "+goose"); ok {
				switch strings.ToLower(strings.Join(strings.Fields(arg), " ")) {
				case "up":
					section = "up"
				case "down":
					section = "down"
				case "statementbegin":
					inBlock = true
				case "statementend":
					inBlock = false
				case "no transaction":
					noTx = true
				case "envsub on", "envsub off":
					// Environment substitution is a goose runtime feature; the SQL stays as written.
				default:
					return nil, nil, false, fmt.Errorf("goose: line %d: unknown annotation %q", n, strings.TrimSpace(line))
				}
				continue
			}
		}
		switch section {
		case "up":
			upBuf.WriteString(line + "\n")
		case "down":
			downBuf.WriteString(line + "\n")
		default:
			if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "--") {
				return nil, nil, false, fmt.Errorf("goose: line %d: SQL before -- +goose Up", n)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, nil, false, fmt.Errorf("goose: %w", err)
	}
	if inBlock {
		return nil, nil, false, errors.New("goose: -- +goose StatementBegin without StatementEnd")
	}
	if section == "" {
		return nil, nil, false, errors.New("goose: no -- +goose Up section")
	}
	up = bytes.TrimSpace(upBuf.Bytes())
	if d := bytes.TrimSpace(downBuf.Bytes()); len(d) > 0 {
		down = d
	}
	return up, down, noTx, nil
}

// Read returns a history.Source. Versions come from the leading number of
// each file name, kept as written ("00001"). NoTx is set from the NO
// TRANSACTION annotation. Applied versions come from the table: the latest
// row per version_id decides, and a version is applied when that row has
// is_applied true; version_id is matched to the file whose leading number
// equals it. Row version_id 0 (goose's initial row) is ignored. A version
// the table lists as applied that has no file is an error: the history would
// not match the directory.
func Read(dir fs.FS, c momo.PinnedConn, table string) (history.Source, error) {
	src := history.Source{Tool: "goose", Applied: []string{}}
	ents, err := fs.ReadDir(dir, ".")
	if err != nil {
		return src, fmt.Errorf("goose: %w", err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".go") {
			src.Unsupported = append(src.Unsupported, n)
			continue
		}
		m := fileRe.FindStringSubmatch(n)
		if m == nil {
			continue
		}
		b, err := fs.ReadFile(dir, n)
		if err != nil {
			return src, fmt.Errorf("goose: %w", err)
		}
		up, down, noTx, err := parseFile(b)
		if err != nil {
			return src, fmt.Errorf("%s: %w", n, err)
		}
		src.Migrations = append(src.Migrations, history.Raw{Version: m[1], Name: m[2], Up: up, Down: down, NoTx: noTx, Origin: n})
	}
	sort.SliceStable(src.Migrations, func(i, j int) bool {
		return histutil.LessNumeric(src.Migrations[i].Version, src.Migrations[j].Version)
	})
	tbl, err := histutil.QuoteTable(table)
	if err != nil {
		return src, err
	}
	rows, err := c.Query(context.Background(), "SELECT version_id, is_applied FROM "+tbl+" ORDER BY id", nil)
	if err != nil {
		return src, fmt.Errorf("goose: read %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	latest := map[int64]bool{}
	for rows.Next() {
		var v int64
		var applied bool
		if err := rows.Scan(&v, &applied); err != nil {
			return src, fmt.Errorf("goose: read %s: %w", table, err)
		}
		latest[v] = applied
	}
	if err := rows.Err(); err != nil {
		return src, fmt.Errorf("goose: read %s: %w", table, err)
	}
	have := map[int64]bool{}
	for _, m := range src.Migrations {
		v, err := strconv.ParseInt(m.Version, 10, 64)
		if err != nil {
			return src, fmt.Errorf("goose: %s: version %q: %w", m.Origin, m.Version, err)
		}
		have[v] = true
		if latest[v] {
			src.Applied = append(src.Applied, m.Version)
		}
	}
	for v, ok := range latest {
		if v != 0 && ok && !have[v] {
			return src, fmt.Errorf("goose: %s lists version %d as applied but the directory has no file for it", table, v)
		}
	}
	return src, nil
}
