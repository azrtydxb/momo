// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

var (
	nextvalCall    = regexp.MustCompile(`(?i)nextval\s*\(`)
	nextvalLiteral = regexp.MustCompile(`(?i)nextval\s*\(\s*'([^']*)'`)
)

// bareSeqName returns the sequence name of a nextval literal without its schema
// and quotes.
func bareSeqName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "."); i >= 0 {
		s = s[i+1:]
	}
	return strings.Trim(s, `"`)
}

// refsSequence reports whether text may call nextval on the sequence: a literal
// that names it, or a nextval whose argument is not a plain literal and so
// cannot be read reliably (which counts as a use, to fail safe).
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

// tableUsesSequence reports whether a column default, a generation expression
// or a check of the table calls nextval on the sequence.
func tableUsesSequence(t *schema.Table, bare string) bool {
	for _, c := range t.Columns {
		if refsSequence(c.Default, bare) || refsSequence(c.Generated, bare) {
			return true
		}
	}
	for _, ck := range t.Checks {
		if refsSequence(ck.Expr, bare) {
			return true
		}
	}
	return false
}

// sequenceHost returns the model that declares a standalone sequence, or nil
// when the sequence is left out. Without introspect.only it is the first model.
// With it, a sequence is kept only when a modeled table uses it (then the first
// such table declares it) or when only names the sequence itself (then the
// first model declares it). A sequence that a column owns is not standalone, so
// inspection never reports it here.
func (g *generator) sequenceHost(q schema.Sequence) *model {
	if len(g.o.Config.Only) == 0 {
		return g.models[0]
	}
	for _, m := range g.models {
		if tableUsesSequence(m.t, q.Name) {
			return m
		}
	}
	if schema.MatchObject(g.o.Config.Only, q.Schema, q.Name) {
		return g.models[0]
	}
	return nil
}
