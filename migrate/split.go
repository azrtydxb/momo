// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"fmt"
	"strings"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// SplitStatements splits a migration body into statements. A statement ends
// at a semicolon outside single-quoted strings, double-quoted identifiers,
// line and block comments and, for PostgreSQL, dollar-quoted bodies. Block
// comments nest for PostgreSQL only. For SQLite a CREATE TRIGGER statement
// ends at the END; that closes its body. A `-- momo:split` directive splits at
// every semicolon outside strings and comments. Statements keep their text,
// including preceding comments, trimmed of surrounding whitespace; statements
// holding only whitespace and comments are dropped. A leading UTF-8 BOM is
// ignored. Offsets in errors count from the start of body.
func SplitStatements(body []byte, dialectName string) ([]string, error) {
	base := 0
	if bytes.HasPrefix(body, utf8BOM) {
		body = body[len(utf8BOM):]
		base = len(utf8BOM)
	}
	// An unknown directive is reported by ParseDirectives, not here.
	dirs, _ := parseDirectives(body)
	sc := &splitter{
		src:      body,
		base:     base,
		pg:       dialectName == "postgres" && !dirs.Split,
		nested:   dialectName == "postgres",
		sqlite:   dialectName == "sqlite",
		mysql:    dialectName == "mysql",
		triggers: dialectName == "sqlite" && !dirs.Split,
	}
	return sc.run()
}

type splitter struct {
	src           []byte
	base          int
	pg, nested    bool
	sqlite, mysql bool
	triggers      bool
	out           []string
	start         int
	words         []string
	trigger       bool
	caseDepth     int
	lastWord      string
}

func isIdent(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func (s *splitter) errAt(what string, i int) error {
	line := 1 + bytes.Count(s.src[:i], []byte{'\n'})
	return fmt.Errorf("migrate: unterminated %s at byte %d (line %d)", what, s.base+i, line)
}

func (s *splitter) resetStmt(next int) {
	s.start = next
	s.words = s.words[:0]
	s.trigger = false
	s.caseDepth = 0
	s.lastWord = ""
}

func (s *splitter) emit(end int) {
	text := strings.TrimSpace(string(s.src[s.start:end]))
	if text != "" && !s.onlyTrivia(text) {
		s.out = append(s.out, text)
	}
}

// onlyTrivia reports whether text holds nothing but whitespace and comments.
// The text was already scanned, so its comments are known to be terminated.
func (s *splitter) onlyTrivia(text string) bool {
	b := []byte(text)
	for i := 0; i < len(b); {
		switch {
		case isSpace(b[i]):
			i++
		case s.lineCommentAt(b, i):
			i = skipLine(b, i)
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			end := s.blockEnd(b, i)
			if end < 0 {
				return false
			}
			i = end
		default:
			return false
		}
	}
	return true
}

func (s *splitter) lineCommentAt(b []byte, i int) bool {
	return b[i] == '-' && i+1 < len(b) && b[i+1] == '-' || s.mysql && b[i] == '#'
}

func skipLine(b []byte, i int) int {
	for i < len(b) && b[i] != '\n' {
		i++
	}
	return i
}

// blockEnd returns the index just past the block comment starting at i, or -1.
func (s *splitter) blockEnd(b []byte, i int) int {
	depth := 1
	j := i + 2
	for j < len(b) {
		switch {
		case b[j] == '*' && j+1 < len(b) && b[j+1] == '/':
			depth--
			j += 2
			if depth == 0 {
				return j
			}
		case s.nested && b[j] == '/' && j+1 < len(b) && b[j+1] == '*':
			depth++
			j += 2
		default:
			j++
		}
	}
	return -1
}

// quoted returns the index just past the quoted run opened at i by q, or -1.
// A doubled quote character stays inside the run; backslash escapes apply when
// esc is true.
func quoted(b []byte, i int, q byte, esc bool) int {
	j := i + 1
	for j < len(b) {
		switch {
		case esc && b[j] == '\\':
			j += 2
		case b[j] == q:
			if j+1 < len(b) && b[j+1] == q {
				j += 2
				continue
			}
			return j + 1
		default:
			j++
		}
	}
	return -1
}

func (s *splitter) run() ([]string, error) {
	b := s.src
	i := 0
	for i < len(b) {
		c := b[i]
		switch {
		case isSpace(c):
			i++
		case s.lineCommentAt(b, i):
			i = skipLine(b, i)
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			end := s.blockEnd(b, i)
			if end < 0 {
				return nil, s.errAt("block comment", i)
			}
			i = end
		case c == '\'' || c == '"':
			esc := s.mysql
			if s.pg && c == '\'' && i > 0 && (b[i-1] == 'e' || b[i-1] == 'E') && (i == 1 || !isIdent(b[i-2])) {
				esc = true
			}
			end := quoted(b, i, c, esc)
			if end < 0 {
				what := "string literal"
				if c == '"' {
					what = "quoted identifier"
				}
				return nil, s.errAt(what, i)
			}
			s.lastWord = ""
			i = end
		case c == '`' && (s.mysql || s.sqlite), c == '[' && s.sqlite:
			closer := byte('`')
			if c == '[' {
				closer = ']'
			}
			end := bytes.IndexByte(b[i+1:], closer)
			if end < 0 {
				return nil, s.errAt("quoted identifier", i)
			}
			s.lastWord = ""
			i += end + 2
		case c == '$' && s.pg && (i == 0 || !isIdent(b[i-1])):
			j := i + 1
			if j < len(b) && !(b[j] >= '0' && b[j] <= '9') {
				for j < len(b) && isIdent(b[j]) && b[j] != '$' {
					j++
				}
			}
			if j < len(b) && b[j] == '$' {
				tag := b[i : j+1]
				end := bytes.Index(b[j+1:], tag)
				if end < 0 {
					return nil, s.errAt("dollar-quoted string", i)
				}
				s.lastWord = ""
				i = j + 1 + end + len(tag)
				continue
			}
			s.lastWord = ""
			i++
		case c == ';':
			if s.trigger && !(s.lastWord == "END" && s.caseDepth == 0) {
				s.lastWord = ""
				i++
				continue
			}
			s.emit(i)
			s.resetStmt(i + 1)
			i++
		case isIdent(c):
			j := i
			for j < len(b) && isIdent(b[j]) {
				j++
			}
			s.word(strings.ToUpper(string(b[i:j])))
			i = j
		default:
			s.lastWord = ""
			i++
		}
	}
	s.emit(len(b))
	return s.out, nil
}

func (s *splitter) word(w string) {
	if len(s.words) < 3 {
		s.words = append(s.words, w)
		if s.triggers && !s.trigger && len(s.words) >= 2 && s.words[0] == "CREATE" {
			if s.words[1] == "TRIGGER" || len(s.words) == 3 && (s.words[1] == "TEMP" || s.words[1] == "TEMPORARY") && s.words[2] == "TRIGGER" {
				s.trigger = true
			}
		}
	}
	s.lastWord = w
	if !s.trigger {
		return
	}
	switch w {
	case "CASE":
		s.caseDepth++
	case "END":
		if s.caseDepth > 0 {
			s.caseDepth--
			s.lastWord = ""
		}
	}
}
