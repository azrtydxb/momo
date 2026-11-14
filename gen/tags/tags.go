// SPDX-License-Identifier: Apache-2.0

// Package tags parses the `momo` struct tag grammar: a comma-separated list of
// `key` or `key:value` options. A value runs from the first colon to the next
// comma, so it cannot contain a comma; values that need commas belong in the
// companion file.
package tags

import (
	"strconv"
	"strings"
)

// Problem is one diagnostic produced while parsing a tag.
type Problem struct{ Msg string }

// Tags is the parsed form of one `momo` struct tag.
type Tags struct {
	Skip bool

	Column, Type, Collate, Default, Check, Comment, Enum, Table, Schema, Prefix       string
	Fk, FkName, Join, OnDelete, OnUpdate, SoftDelete, TimeUnit, IndexName, UniqueName string
	Sort, Nulls, Method, View                                                         string

	// Generated is the generated option: the expression of a stored generated
	// column (GENERATED ALWAYS AS (expr) STORED). It needs read_only.
	Generated string

	// GeneratedVirtual is the virtual option: the generated column is VIRTUAL
	// (computed on read) instead of STORED. It is valid only together with
	// generated (or the MomoConfig Generated hooks), which BuildSchema checks.
	GeneratedVirtual bool

	// IDGen is the idgen option: the name of a Go-side id generator.
	IDGen string

	Materialized bool

	Rel, PolyType, PolyValue string

	Size, Precision, Scale, Dim int

	// IdentityAlways is the identity:always option: GENERATED ALWAYS AS
	// IDENTITY. It implies Identity.
	IdentityAlways bool

	PK, NotNull, Nullable, Identity, Serial, Version, CreatedAt, UpdatedAt bool
	ReadOnly, CreateOnly, Immutable, Sensitive, Embedded                   bool
	NoConstraint, BelongsTo, HasOne, HasMany, ManyToMany                   bool
	Index, Unique                                                          bool

	// DBDefault is the dbdefault option: a Model insert leaves the column to
	// the database default while the field holds its zero value.
	DBDefault bool

	// Autoincrement, WithoutRowID and Strict are the SQLite table options of
	// the momo.Model marker: the INTEGER PRIMARY KEY is declared AUTOINCREMENT,
	// the table is WITHOUT ROWID, the table is STRICT. They mean nothing on a
	// field.
	Autoincrement, WithoutRowID, Strict bool
}

// Parse parses a raw tag value. An empty tag yields the zero Tags and no
// problems; "-" yields Tags{Skip: true}.
func Parse(raw string) (Tags, []Problem) {
	var t Tags
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return t, nil
	}
	if trimmed == "-" {
		t.Skip = true
		return t, nil
	}
	var probs []Problem
	seen := map[string]bool{}
	for _, part := range SplitOptions(raw) {
		key, val, hasVal := strings.Cut(strings.TrimSpace(part), ":")
		msg := ""
		if seen[key] && key != "" {
			msg = `duplicate tag option "` + key + `"`
		} else if m := t.set(key, val, hasVal); m != "" {
			msg = m
		}
		seen[key] = true
		if msg != "" {
			probs = append(probs, Problem{Msg: msg})
		}
	}
	return t, probs
}

// SplitOptions splits a raw tag value into its options at the commas outside
// parentheses, so type:decimal(10,2) and type:enum('a','b') are one option
// each. Inside parentheses a single-quoted literal is skipped, with ” as its
// escape, so a parenthesis or comma in an enum value does not count. A value
// whose parentheses or quotes do not balance is split at every comma, as a tag
// without parentheses always is.
func SplitOptions(raw string) []string {
	var out []string
	depth, start := 0, 0
	quoted := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if quoted {
			if c == '\'' {
				if i+1 < len(raw) && raw[i+1] == '\'' {
					i++
				} else {
					quoted = false
				}
			}
			continue
		}
		switch c {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return strings.Split(raw, ",")
			}
			depth--
		case '\'':
			if depth > 0 {
				quoted = true
			}
		case ',':
			if depth == 0 {
				out = append(out, raw[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 || quoted {
		return strings.Split(raw, ",")
	}
	return append(out, raw[start:])
}

// set applies one option and returns the problem message, or "" on success.
func (t *Tags) set(key, val string, hasVal bool) string {
	bare := func(dst *bool) string {
		if hasVal {
			return key + " does not take a value"
		}
		*dst = true
		return ""
	}
	str := func(dst *string) string {
		if !hasVal || val == "" {
			return key + " requires a value"
		}
		*dst = val
		return ""
	}
	num := func(dst *int) string {
		if !hasVal || val == "" {
			return key + " requires a value"
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			return key + " must be an integer"
		}
		if n < 0 {
			return key + " must not be negative"
		}
		*dst = n
		return ""
	}
	enum := func(dst *string, allowed ...string) string {
		if !hasVal || val == "" {
			return key + " requires a value"
		}
		for _, a := range allowed {
			if val == a {
				*dst = val
				return ""
			}
		}
		return key + " must be " + joinOr(allowed)
	}
	stamp := func(dst *bool) string {
		switch val {
		case "unix", "unixmilli", "unixnano":
		case "":
			if hasVal {
				return key + " must be unix, unixmilli or unixnano"
			}
		default:
			return key + " must be unix, unixmilli or unixnano"
		}
		*dst = true
		t.TimeUnit = val
		return ""
	}
	named := func(dst *bool, name *string) string {
		if hasVal {
			if val == "" {
				return key + " requires a value"
			}
			*name = val
		}
		*dst = true
		return ""
	}
	switch key {
	case "pk":
		return bare(&t.PK)
	case "identity":
		switch {
		case !hasVal:
			t.Identity = true
		case val == "always":
			t.Identity, t.IdentityAlways = true, true
		case val == "by_default":
			t.Identity = true
		default:
			return "identity must be bare, identity:always or identity:by_default"
		}
		return ""
	case "serial":
		return bare(&t.Serial)
	case "dbdefault":
		return bare(&t.DBDefault)
	case "autoincrement":
		return bare(&t.Autoincrement)
	case "without_rowid":
		return bare(&t.WithoutRowID)
	case "strict":
		return bare(&t.Strict)
	case "version":
		return bare(&t.Version)
	case "notnull":
		return bare(&t.NotNull)
	case "nullable":
		return bare(&t.Nullable)
	case "read_only":
		return bare(&t.ReadOnly)
	case "create_only":
		return bare(&t.CreateOnly)
	case "immutable":
		return bare(&t.Immutable)
	case "sensitive":
		return bare(&t.Sensitive)
	case "embedded":
		return bare(&t.Embedded)
	case "no_constraint":
		return bare(&t.NoConstraint)
	case "belongs_to":
		return bare(&t.BelongsTo)
	case "has_one":
		return bare(&t.HasOne)
	case "has_many":
		return bare(&t.HasMany)
	case "many2many":
		return bare(&t.ManyToMany)
	case "materialized":
		return bare(&t.Materialized)
	case "created_at":
		return stamp(&t.CreatedAt)
	case "updated_at":
		return stamp(&t.UpdatedAt)
	case "column":
		return str(&t.Column)
	case "type":
		return str(&t.Type)
	case "collate":
		return str(&t.Collate)
	case "default":
		return str(&t.Default)
	case "check":
		return str(&t.Check)
	case "comment":
		return str(&t.Comment)
	case "enum":
		return str(&t.Enum)
	case "table":
		return str(&t.Table)
	case "schema":
		return str(&t.Schema)
	case "view":
		return str(&t.View)
	case "prefix":
		return str(&t.Prefix)
	case "idgen":
		return str(&t.IDGen)
	case "generated":
		return str(&t.Generated)
	case "virtual":
		return bare(&t.GeneratedVirtual)
	case "fk":
		return str(&t.Fk)
	case "fk_name":
		return str(&t.FkName)
	case "join":
		return str(&t.Join)
	case "on_delete":
		return str(&t.OnDelete)
	case "on_update":
		return str(&t.OnUpdate)
	case "method":
		return str(&t.Method)
	case "polytype":
		return str(&t.PolyType)
	case "polyvalue":
		return str(&t.PolyValue)
	case "rel":
		return enum(&t.Rel, "belongs_to", "has_one", "has_many", "many2many")
	case "soft_delete":
		return enum(&t.SoftDelete, "timestamp", "unix", "bool")
	case "sort":
		return enum(&t.Sort, "asc", "desc")
	case "nulls":
		return enum(&t.Nulls, "first", "last")
	case "size":
		return num(&t.Size)
	case "precision":
		return num(&t.Precision)
	case "scale":
		return num(&t.Scale)
	case "dim":
		return num(&t.Dim)
	case "index":
		return named(&t.Index, &t.IndexName)
	case "unique":
		return named(&t.Unique, &t.UniqueName)
	}
	return `unknown tag option "` + key + `"`
}

func joinOr(a []string) string {
	if len(a) == 2 {
		return a[0] + " or " + a[1]
	}
	return strings.Join(a[:len(a)-1], ", ") + " or " + a[len(a)-1]
}
