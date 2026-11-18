// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

var (
	nextvalCall    = regexp.MustCompile(`(?i)\b(?:nextval|currval|setval)\s*\(`)
	nextvalLiteral = regexp.MustCompile(`(?i)\b(?:nextval|currval|setval)\s*\(\s*'((?:[^']|'')*)'`)
)

// bareSeqName returns the sequence name of a nextval literal without its schema
// and quotes. A dot inside double quotes belongs to the name, and a doubled
// quote inside them is one quote.
func bareSeqName(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "''", "'"))
	var cur strings.Builder
	quoted := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' && quoted && i+1 < len(s) && s[i+1] == '"':
			cur.WriteByte('"')
			i++
		case c == '"':
			quoted = !quoted
		case c == '.' && !quoted:
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return cur.String()
}

// refsSequence reports whether text may call nextval on the sequence: a literal
// that names it, or a call whose argument is not a plain literal and so cannot
// be read reliably (which counts as a use, to fail safe). The schema of a
// qualified literal is ignored, which keeps a sequence when two schemas share
// a name.
func refsSequence(text, bare string) bool {
	calls := len(nextvalCall.FindAllStringIndex(text, -1))
	if calls == 0 {
		return false
	}
	lits := nextvalLiteral.FindAllStringSubmatch(text, -1)
	if len(lits) != calls {
		return true
	}
	for _, m := range lits {
		if strings.EqualFold(bareSeqName(m[1]), bare) {
			return true
		}
	}
	return false
}

func anyRefsSequence(texts []string, bare string) bool {
	for _, t := range texts {
		if refsSequence(t, bare) {
			return true
		}
	}
	return false
}

// sequenceHost returns the model that declares a standalone sequence, or nil
// when the sequence is left out. Without introspect.only it is the first model.
// With it, a sequence is kept only when something that is written uses it: a
// column default, generated expression, check, index expression or predicate,
// trigger or policy of a modeled table, or a function or view that is declared
// (claimed holds the model of each function, as claimFunctions returns it).
// The first model that has such a use declares it, and a view alone puts it on
// the first model. A sequence that only names itself goes on the first model. A
// sequence that a column owns is not standalone, so inspection never reports it
// here.
func (g *generator) sequenceHost(q schema.Sequence, claimed []*model) *model {
	if len(g.o.Config.Only) == 0 {
		return g.models[0]
	}
	for _, m := range g.models {
		if anyRefsSequence(g.tableTexts(m), q.Name) {
			return m
		}
		for i, f := range g.clean.Functions {
			if claimed[i] == m && anyRefsSequence([]string{f.Body, f.Args}, q.Name) {
				return m
			}
		}
	}
	for _, v := range g.clean.Views {
		if refsSequence(v.Query, q.Name) {
			return g.models[0]
		}
	}
	if schema.MatchObject(g.o.Config.Only, q.Schema, q.Name) {
		return g.models[0]
	}
	return nil
}
