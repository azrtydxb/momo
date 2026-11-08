// SPDX-License-Identifier: Apache-2.0

package bun

import "strings"

// tag is a parsed bun struct tag. The parser below is a port of
// internal/tagparser in github.com/uptrace/bun v1.3.0, so quoting, the
// parentheses rule and the "first bare word is the name" rule behave exactly
// as they do in Bun.
type tag struct {
	name string
	opts map[string][]string
}

// zero reports whether the tag is empty, as Bun's Tag.IsZero does.
func (t tag) zero() bool { return t.name == "" && t.opts == nil }

func (t tag) has(key string) bool {
	_, ok := t.opts[key]
	return ok
}

// option returns the last value of key, as Bun's Tag.Option does.
func (t tag) option(key string) (string, bool) {
	if vs, ok := t.opts[key]; ok {
		return vs[len(vs)-1], true
	}
	return "", false
}

func parseTag(s string) tag {
	if s == "" {
		return tag{}
	}
	p := &tagParser{s: s}
	p.parse()
	return p.tag
}

type tagParser struct {
	s        string
	i        int
	tag      tag
	seenName bool
}

func (p *tagParser) setName(name string) {
	if p.seenName {
		p.addOption(name, "")
		return
	}
	p.seenName = true
	p.tag.name = name
}

func (p *tagParser) addOption(key, value string) {
	p.seenName = true
	if key == "" {
		return
	}
	if p.tag.opts == nil {
		p.tag.opts = map[string][]string{}
	}
	p.tag.opts[key] = append(p.tag.opts[key], value)
}

func (p *tagParser) parse() {
	for p.valid() {
		p.parseKeyValue()
		if p.peek() == ',' {
			p.i++
		}
	}
}

func (p *tagParser) parseKeyValue() {
	start := p.i
	for p.valid() {
		switch p.read() {
		case ',':
			p.setName(p.s[start : p.i-1])
			return
		case ':':
			key := p.s[start : p.i-1]
			p.addOption(key, p.parseValue())
			return
		case '"':
			p.setName(p.parseQuotedValue())
			return
		}
	}
	p.setName(p.s[start:p.i])
}

func (p *tagParser) parseValue() string {
	start := p.i
	for p.valid() {
		switch p.read() {
		case '"':
			return p.parseQuotedValue()
		case ',':
			return p.s[start : p.i-1]
		case '(':
			p.skipPairs('(', ')')
		}
	}
	if p.i == start {
		return ""
	}
	return p.s[start:p.i]
}

func (p *tagParser) parseQuotedValue() string {
	if i := strings.IndexByte(p.s[p.i:], '"'); i >= 0 && p.s[p.i+i-1] != '\\' {
		s := p.s[p.i : p.i+i]
		p.i += i + 1
		return s
	}
	b := make([]byte, 0, 16)
	for p.valid() {
		switch c := p.read(); c {
		case '\\':
			b = append(b, p.read())
		case '"':
			return string(b)
		default:
			b = append(b, c)
		}
	}
	return ""
}

func (p *tagParser) skipPairs(start, end byte) {
	level := 0
	for p.valid() {
		switch c := p.read(); c {
		case '"':
			_ = p.parseQuotedValue()
		case start:
			level++
		case end:
			if level == 0 {
				return
			}
			level--
		}
	}
}

func (p *tagParser) valid() bool { return p.i < len(p.s) }

func (p *tagParser) read() byte {
	if !p.valid() {
		return 0
	}
	c := p.s[p.i]
	p.i++
	return c
}

func (p *tagParser) peek() byte {
	if !p.valid() {
		return 0
	}
	return p.s[p.i]
}
