// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

// Composite is a PostgreSQL composite type, with the exported fields of the
// struct T as its attributes in declaration order:
//
//	type Address struct {
//		Street string
//		Zip    *string // NULL when nil
//	}
//
//	Home types.Composite[Address] `momo:"type:address"`
//
// The column is declared with the type tag naming the composite, which a
// MomoConfig declares with Config.Composite. Value and Scan speak the record
// text PostgreSQL prints, such as (Main St,"12 34"), with the quoting, the
// NULL fields and the empty strings that format has, so the type works with
// pgx and with database/sql alike.
//
// The fields of T map to the attributes by position, not by name. A field is
// one of: bool, a sized or unsized integer, float32, float64, string, []byte
// (bytea), time.Time (the instant, as a timestamp with a time zone), another
// codec of this package or any driver.Valuer and sql.Scanner pair such as
// sql.NullString, or a pointer to one of these, where nil is NULL. A NULL
// attribute cannot be read into a field that is not a pointer (the field
// should be one): the scan fails. T has no unexported or embedded field and
// no nested struct other than those listed. Value and Scan return an error for
// a T that does not fit these rules, and for a record whose number of fields
// differs from the number of fields of T.
//
// Scan(nil) is an error: use a *Composite[T] for a nullable column. A nested
// composite type and an array of composites are not supported.
//
// Composite types exist on PostgreSQL alone. The generator refuses a Composite
// field when the target dialect is SQLite, MySQL or MariaDB.
type Composite[T any] struct {
	V T
}

// SQLType returns the empty string: the name of a composite type is declared by
// the model (the type tag) and is not a property of the Go type.
func (Composite[T]) SQLType(dialect.Name) string { return "" }

// Value encodes V as PostgreSQL record text.
func (c Composite[T]) Value() (driver.Value, error) {
	v := reflect.ValueOf(&c.V).Elem()
	fields, err := compositeFields(v.Type())
	if err != nil {
		return nil, err
	}
	texts := make([]*string, len(fields))
	for i, idx := range fields {
		t, err := encodeField(v.Field(idx))
		if err != nil {
			return nil, fmt.Errorf("types: encode Composite[%s] field %s: %w", v.Type(), v.Type().Field(idx).Name, err)
		}
		texts[i] = t
	}
	return FormatRecord(texts), nil
}

// String returns the record text Value writes, or the error text when V does
// not fit the rules of Composite.
func (c Composite[T]) String() string {
	v, err := c.Value()
	if err != nil {
		return err.Error()
	}
	return v.(string)
}

// Scan decodes PostgreSQL record text (string or []byte) into V. NULL is an
// error.
func (c *Composite[T]) Scan(src any) error {
	s, err := text("Composite", src)
	if err != nil {
		return err
	}
	texts, err := ParseRecord(s)
	if err != nil {
		return err
	}
	v := reflect.ValueOf(&c.V).Elem()
	fields, err := compositeFields(v.Type())
	if err != nil {
		return err
	}
	if len(texts) != len(fields) {
		return fmt.Errorf("types: Composite[%s] has %d fields but the record %q has %d", v.Type(), len(fields), s, len(texts))
	}
	var out T
	ov := reflect.ValueOf(&out).Elem()
	for i, idx := range fields {
		if err := decodeField(ov.Field(idx), texts[i]); err != nil {
			return fmt.Errorf("types: decode Composite[%s] field %s: %w", v.Type(), v.Type().Field(idx).Name, err)
		}
	}
	c.V = out
	return nil
}

// FormatRecord writes the fields as PostgreSQL record text: a parenthesized,
// comma-separated list in which a nil field is NULL (nothing between the
// commas) and every other field is quoted only when the text would otherwise
// be misread: when it is empty (an empty string is "", not NULL) or contains a
// quote, a backslash, a parenthesis, a comma or white space. A quote and a
// backslash inside quotes are doubled, the way PostgreSQL prints them. ParseRecord
// reads it back.
func FormatRecord(fields []*string) string {
	var sb strings.Builder
	sb.WriteByte('(')
	for i, f := range fields {
		if i > 0 {
			sb.WriteByte(',')
		}
		if f != nil {
			sb.WriteString(quoteRecordField(*f))
		}
	}
	sb.WriteByte(')')
	return sb.String()
}

// recordSpace reports the white space PostgreSQL's record parser keeps inside
// a field and its printer quotes.
func recordSpace(c rune) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// quoteRecordField double-quotes s when record text would otherwise misread
// it. An empty string is quoted, because an empty unquoted field is NULL.
func quoteRecordField(s string) string {
	plain := s != ""
	for i := 0; plain && i < len(s); i++ {
		switch c := s[i]; {
		case c == '"', c == '\\', c == '(', c == ')', c == ',', recordSpace(rune(c)):
			plain = false
		}
	}
	if plain {
		return s
	}
	return `"` + strings.NewReplacer(`"`, `""`, `\`, `\\`).Replace(s) + `"`
}

// ParseRecord reads PostgreSQL record text into its fields, with nil for a
// NULL one: an unquoted empty field. A quoted empty field is the empty string.
// A field may be double-quoted, with "" or \" for a quote and \\ for a
// backslash inside, and a backslash outside quotes escapes the next
// character; white space outside the parentheses is ignored and white space
// inside is part of a field. A record with one NULL field is "()", so a record
// always has at least one field.
func ParseRecord(s string) ([]*string, error) {
	t := strings.TrimFunc(s, recordSpace)
	if len(t) < 2 || t[0] != '(' || t[len(t)-1] != ')' {
		return nil, fmt.Errorf("types: invalid record %q: want text in parentheses", s)
	}
	body := t[1 : len(t)-1]
	var out []*string
	var cur strings.Builder
	quoted, inQuote := false, false
	flush := func() {
		if quoted || cur.Len() > 0 {
			v := cur.String()
			out = append(out, &v)
		} else {
			out = append(out, nil)
		}
		cur.Reset()
		quoted = false
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '\\':
			i++
			if i >= len(body) {
				return nil, fmt.Errorf("types: invalid record %q: a backslash ends the text", s)
			}
			cur.WriteByte(body[i])
		case inQuote && c == '"':
			if i+1 < len(body) && body[i+1] == '"' {
				cur.WriteByte('"')
				i++
			} else {
				inQuote = false
			}
		case inQuote:
			cur.WriteByte(c)
		case c == '"':
			inQuote, quoted = true, true
		case c == ',':
			flush()
		case c == '(' || c == ')':
			return nil, fmt.Errorf("types: invalid record %q: an unquoted parenthesis inside the fields", s)
		default:
			cur.WriteByte(c)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("types: invalid record %q: an unterminated quote", s)
	}
	flush()
	return out, nil
}

var (
	valuerType  = reflect.TypeFor[driver.Valuer]()
	scannerType = reflect.TypeFor[sql.Scanner]()
	timeType    = reflect.TypeFor[time.Time]()
)

// compositeFields returns the field indexes of the struct type t, which all
// have to be exported.
func compositeFields(t reflect.Type) ([]int, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("types: Composite[%s]: the type parameter must be a struct", t)
	}
	if t.NumField() == 0 {
		return nil, fmt.Errorf("types: Composite[%s]: the struct has no fields", t)
	}
	idx := make([]int, t.NumField())
	for i := range idx {
		f := t.Field(i)
		if !f.IsExported() || f.Anonymous {
			return nil, fmt.Errorf("types: Composite[%s]: field %s must be exported and not embedded", t, f.Name)
		}
		idx[i] = i
	}
	return idx, nil
}

// encodeField returns the record text of one field, nil for NULL.
func encodeField(v reflect.Value) (*string, error) {
	if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil() {
		return nil, nil
	}
	switch {
	case v.Type().Implements(valuerType):
		return valuerRecordText(v.Interface().(driver.Valuer))
	case v.CanAddr() && reflect.PointerTo(v.Type()).Implements(valuerType):
		return valuerRecordText(v.Addr().Interface().(driver.Valuer))
	case v.Type() == timeType:
		return strPtr(v.Interface().(time.Time).UTC().Format(zoneMinute)), nil
	}
	switch v.Kind() {
	case reflect.Pointer:
		return encodeField(v.Elem())
	case reflect.Bool:
		return strPtr(boolText(v.Bool())), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strPtr(strconv.FormatInt(v.Int(), 10)), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strPtr(strconv.FormatUint(v.Uint(), 10)), nil
	case reflect.Float32:
		return strPtr(floatText(v.Float(), 32)), nil
	case reflect.Float64:
		return strPtr(floatText(v.Float(), 64)), nil
	case reflect.String:
		return strPtr(v.String()), nil
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			if v.IsNil() {
				return nil, nil
			}
			return strPtr(`\x` + hex.EncodeToString(v.Bytes())), nil
		}
	}
	return nil, fmt.Errorf("the type %s has no record text form", v.Type())
}

// valuerRecordText is the record text of a driver.Valuer: its value as text,
// nil for NULL.
func valuerRecordText(v driver.Valuer) (*string, error) {
	out, err := v.Value()
	if err != nil {
		return nil, err
	}
	switch x := out.(type) {
	case nil:
		return nil, nil
	case string:
		return &x, nil
	case []byte:
		return strPtr(string(x)), nil
	case int64:
		return strPtr(strconv.FormatInt(x, 10)), nil
	case float64:
		return strPtr(floatText(x, 64)), nil
	case bool:
		return strPtr(boolText(x)), nil
	case time.Time:
		return strPtr(x.UTC().Format(zoneMinute)), nil
	}
	return nil, fmt.Errorf("%T encodes as %T, which has no record text form", v, out)
}

func strPtr(s string) *string { return &s }

func boolText(b bool) string {
	if b {
		return "t"
	}
	return "f"
}

// floatText spells a float the way PostgreSQL reads it, with its special
// values as Infinity, -Infinity and NaN.
func floatText(f float64, bits int) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	return strconv.FormatFloat(f, 'g', -1, bits)
}

// decodeField sets v, which is addressable, from the record text s, where nil
// is NULL.
func decodeField(v reflect.Value, s *string) error {
	if s == nil {
		switch {
		case v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface:
			v.SetZero()
			return nil
		case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 && !v.Addr().Type().Implements(scannerType):
			v.SetZero()
			return nil
		case v.Addr().Type().Implements(scannerType):
			return v.Addr().Interface().(sql.Scanner).Scan(nil)
		}
		return fmt.Errorf("NULL cannot be read into the type %s: use a pointer", v.Type())
	}
	if v.Kind() == reflect.Pointer {
		p := reflect.New(v.Type().Elem())
		if err := decodeField(p.Elem(), s); err != nil {
			return err
		}
		v.Set(p)
		return nil
	}
	if v.Addr().Type().Implements(scannerType) {
		return v.Addr().Interface().(sql.Scanner).Scan(*s)
	}
	if v.Type() == timeType {
		t, err := parseRecordTime(*s)
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(t))
		return nil
	}
	switch v.Kind() {
	case reflect.Bool:
		switch strings.ToLower(*s) {
		case "t", "true", "y", "yes", "on", "1":
			v.SetBool(true)
		case "f", "false", "n", "no", "off", "0":
			v.SetBool(false)
		default:
			return fmt.Errorf("%q is not a boolean", *s)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(*s, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a %s: %w", *s, v.Type(), err)
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(*s, 10, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a %s: %w", *s, v.Type(), err)
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(*s, v.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a %s: %w", *s, v.Type(), err)
		}
		v.SetFloat(f)
	case reflect.String:
		v.SetString(*s)
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.Uint8 {
			return fmt.Errorf("the type %s has no record text form", v.Type())
		}
		hexText, ok := strings.CutPrefix(*s, `\x`)
		if !ok {
			return fmt.Errorf("%q is not bytea hex text", *s)
		}
		b, err := hex.DecodeString(hexText)
		if err != nil {
			return fmt.Errorf("%q is not bytea hex text: %w", *s, err)
		}
		v.SetBytes(b)
	default:
		return fmt.Errorf("the type %s has no record text form", v.Type())
	}
	return nil
}

// parseRecordTime reads a timestamp field: with a time zone as an instant, and
// without one as the same wall clock in UTC.
func parseRecordTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{zoneHour, zoneMinute, zoneSecond, time.RFC3339Nano, timestampLayout} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, errors.New("not a timestamp: " + strconv.Quote(s))
}
