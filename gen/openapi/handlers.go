// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/apiquery"
	"github.com/azrtydxb/momo/gen/internal/gosrc"
)

var jsonKeyRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// handlerGoTypes are the Go field types the generated handlers carry through
// JSON unchanged; a pointer to one of them is nullable.
var handlerGoTypes = map[string]bool{
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"string": true, "bool": true, "float32": true, "float64": true,
	"time.Time": true, "[]byte": true, "vector.Vector": true,
}

// hfield is one exposed column of a model, with the Go field that holds it.
type hfield struct {
	f     *gen.Field
	ident string // the name of the field's descriptor in the descriptor package
	// emptyVec is set for a non-pointer vector, which the database cannot
	// store when empty: the handlers leave such a column out of an insert and
	// reset it on an update.
	emptyVec bool
	required bool
	// settable is false for the primary key and for identity and read-only
	// columns, which a request never writes.
	settable bool
	// defaulted is set for a column the database fills in when it is omitted.
	defaulted bool
	// updatable is set for a column a PUT can assign: writable, and none of the
	// version, soft-delete and timestamp columns momo owns.
	updatable bool
}

type hmodel struct {
	m          *gen.Model
	modelAlias string
	descAlias  string
	fields     []hfield
	pk         hfield
	ops        apiquery.OpSet
}

// handlerFields returns the exposed columns of m, the same set the document
// describes, and a diagnostic for a column the handlers cannot carry.
func handlerFields(g *gen.Graph, m *gen.Model) ([]hfield, []gen.Diagnostic) {
	idents, _ := gen.DescriptorIdents(m, g)
	pk := gosrc.SinglePK(g, m)
	var out []hfield
	var diags []gen.Diagnostic
	seen := map[string]bool{}
	for i, f := range m.Fields {
		if f.Tags.Sensitive || seen[f.Column] {
			continue
		}
		seen[f.Column] = true
		if _, ok := schemaFor(f.BaseType, f.Nullable); !ok {
			continue // Document reports it
		}
		switch {
		case len(f.Path) != 1:
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: "OpenAPI handlers do not support the embedded field " + f.GoName + " of " + m.GoName})
		case !handlerGoTypes[strings.TrimPrefix(f.GoType, "*")]:
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: "OpenAPI handlers do not support the type " + f.GoType + " of " + m.GoName + "." + f.GoName})
		case !jsonKeyRE.MatchString(f.Column):
			diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: fmt.Sprintf("column %q of %s is not a plain JSON key", f.Column, m.GoName)})
		default:
			isPK := f == pk
			out = append(out, hfield{
				f: f, ident: idents[i], emptyVec: f.GoType == "vector.Vector",
				required:  requiredOnWrite(f, isPK),
				settable:  !isPK && !f.Tags.Identity && !f.Tags.Serial && f.Access != gen.ReadOnly,
				defaulted: f.Tags.Default != "",
				updatable: !isPK && !f.Tags.Identity && !f.Tags.Serial && f.Access == gen.Writable &&
					!f.Tags.Version && f.Tags.SoftDelete == "" && !f.Tags.CreatedAt && !f.Tags.UpdatedAt,
			})
		}
	}
	return out, diags
}

// Handlers returns the Go source of net/http handlers for the models of g, in
// package pkg, and the diagnostics for what it left out. Each model with a
// single int64 primary key gets a type MHandler with a DB field and a Routes
// method that registers the routes of its enabled operations (Options.Ops) on
// a *http.ServeMux; a model that cannot be served is skipped with a
// diagnostic, and the rest is still generated. The source is nil only when the
// package name or base path is invalid.
//
// The handlers use only the typed builders, filter through the generated
// ColumnByName allowlist, page with keyset cursors on the primary key, accept
// application/json bodies of at most 1 MiB, and answer errors as
// {"error": "..."} with fixed messages: a unique or foreign key violation is
// 409, a check or not-null violation 400, a busy database or a serialization
// failure 503. PUT needs every required property; a property that is absent
// and not required keeps its stored value. They do no authentication or
// authorization: wrap the mux. The response keys are exactly the column
// properties of the model's schema. The output is deterministic.
func Handlers(g *gen.Graph, pkg string, opts Options) ([]byte, []gen.Diagnostic) {
	var diags []gen.Diagnostic
	if !gosrc.PkgName.MatchString(pkg) {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid Go package name %q for the OpenAPI handlers", pkg)}}
	}
	base := strings.TrimRight(opts.BasePath, "/")
	if !gosrc.BasePath.MatchString(base) {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("OpenAPI BasePath %q must be empty or slash-separated segments of letters, digits and . _ ~ -", opts.BasePath)}}
	}
	if err := opts.Ops.Validate(); err != nil {
		return nil, []gen.Diagnostic{{Msg: "OpenAPI " + err.Error()}}
	}

	aliases := gosrc.NewAliases(map[string]string{
		"bytes": "bytes", "json": "encoding/json", "errors": "errors", "io": "io", "mime": "mime",
		"http": "net/http", "sort": "sort", "strconv": "strconv", "strings": "strings", "time": "time",
		"momo": "github.com/azrtydxb/momo", "vector": "github.com/azrtydxb/momo/vector",
		"apiquery": "github.com/azrtydxb/momo/gen/apiquery",
	})

	var models []hmodel
	var notes []string
	seenName := map[string]bool{}
	seenTable := map[string]bool{}
	skip := func(m *gen.Model, why string) {
		notes = append(notes, "// Not served: "+m.GoName+" ("+why+")")
		diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: "OpenAPI handlers skip " + m.GoName + ": " + why})
	}
	for _, m := range gosrc.SortedModels(g) {
		if seenName[m.GoName] || m.GoName == "Error" || !schemaNameRE.MatchString(m.GoName) || !pathSegmentRE.MatchString(m.Table) || seenTable[m.Table] {
			continue // Document reports it
		}
		seenName[m.GoName] = true
		seenTable[m.Table] = true
		fields, fd := handlerFields(g, m)
		if len(fd) > 0 {
			diags = append(diags, fd...)
			notes = append(notes, "// Not served: "+m.GoName+" ("+fd[0].Msg+")")
			continue
		}
		pkf := servedPK(g, m)
		if pkf == nil {
			skip(m, "no single exposed int64 primary key")
			continue
		}
		i := slices.IndexFunc(fields, func(f hfield) bool { return f.f == pkf })
		if i < 0 {
			skip(m, "the primary key is not exposed")
			continue
		}
		pk := fields[i]
		mp, err := gosrc.ModelImport(m)
		if err != nil {
			skip(m, err.Error())
			continue
		}
		dp, err := gosrc.DescImport(m)
		if err != nil {
			skip(m, err.Error())
			continue
		}
		models = append(models, hmodel{
			m: m, fields: fields, pk: pk, ops: opts.Ops.For(m.GoName),
			modelAlias: aliases.Alias(mp, m.PkgName),
			descAlias:  aliases.Alias(dp, m.SubPkg+"desc"),
		})
	}

	var b strings.Builder
	b.WriteString(preamble)
	for _, hm := range models {
		writeHandler(&b, hm, base)
	}
	body := b.String()

	var out strings.Builder
	out.WriteString(gen.Header() + "\n\n")
	for _, n := range notes {
		out.WriteString(n + "\n")
	}
	out.WriteString("// Package " + pkg + " holds net/http handlers for the models, generated from the\n// same graph as the OpenAPI document. The handlers do no authentication or\n// authorization: wrap the mux.\npackage " + pkg + "\n\n")
	out.WriteString(gosrc.Imports(body, aliases.Candidates()))
	out.WriteString(body)
	return []byte(out.String()), diags
}

const preamble = `// maxBodyBytes is the largest request body a handler reads: 1 MiB.
const maxBodyBytes = 1 << 20

// errorBody is the JSON form of every error response.
type errorBody struct {
	Error string ` + "`json:\"error\"`" + `
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError answers a failed statement with a fixed message: the error of the
// database, which names tables and constraints, never reaches the client.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, momo.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not found"})
	case errors.Is(err, momo.ErrUniqueViolation):
		writeJSON(w, http.StatusConflict, errorBody{Error: "conflict: a unique constraint refused the change"})
	case errors.Is(err, momo.ErrForeignKeyViolation):
		writeJSON(w, http.StatusConflict, errorBody{Error: "conflict: a foreign key constraint refused the change"})
	case errors.Is(err, momo.ErrCheckViolation), errors.Is(err, momo.ErrNotNullViolation):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "a constraint refused the value"})
	case errors.Is(err, momo.ErrBusy), errors.Is(err, momo.ErrSerialization):
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "temporarily unavailable; retry"})
	case errors.Is(err, momo.ErrValidation):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "invalid request"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal error"})
	}
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, errorBody{Error: msg})
}

// readBody reads a request body: application/json only (415 otherwise), at
// most maxBodyBytes (413), and exactly one JSON object (400, trailing data
// included). It returns the object's keys and the raw bytes.
func readBody(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, []byte, bool) {
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, errorBody{Error: "content type must be application/json"})
		return nil, nil, false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorBody{Error: "request body too large"})
		} else {
			badRequest(w, "invalid request body")
		}
		return nil, nil, false
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil || keys == nil {
		badRequest(w, "request body must be one JSON object")
		return nil, nil, false
	}
	return keys, data, true
}

// decodeDTO decodes data into v and refuses a key v does not have.
func decodeDTO(w http.ResponseWriter, data []byte, v any) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		badRequest(w, "invalid request body")
		return false
	}
	return true
}

// checkKeys answers 400 when a key is not exactly the name of a property (the
// JSON decoder would match "Title" to "title", and the value would then be
// ignored), a required property is missing or a not-null property is null.
func checkKeys(w http.ResponseWriter, keys map[string]json.RawMessage, known map[string]bool, required []string, notNull map[string]bool) bool {
	for k := range keys {
		if !known[k] {
			badRequest(w, "unknown property")
			return false
		}
	}
	for _, k := range required {
		if _, ok := keys[k]; !ok {
			badRequest(w, "missing required property "+k)
			return false
		}
	}
	for k, v := range keys {
		if notNull[k] && string(bytes.TrimSpace(v)) == "null" {
			badRequest(w, "property "+k+" must not be null")
			return false
		}
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		badRequest(w, "id must be an integer")
		return 0, false
	}
	return id, true
}
`

func writeHandler(b *strings.Builder, hm hmodel, base string) {
	m := hm.m
	r := strings.NewReplacer(
		"{{M}}", m.GoName,
		"{{lm}}", gosrc.LowerFirst(m.GoName),
		"{{models}}", hm.modelAlias,
		"{{desc}}", hm.descAlias,
		"{{PK}}", hm.pk.f.GoName,
		"{{PKI}}", hm.pk.ident,
		"{{base}}", base,
		"{{table}}", m.Table,
	)
	has := hm.ops.Has
	write := has(apiquery.OpCreate) || has(apiquery.OpUpdate)

	b.WriteString(r.Replace(`
// {{M}}Handler serves the {{M}} endpoints of the OpenAPI document.
type {{M}}Handler struct{ DB *momo.DB }

// Routes registers the {{M}} routes on mux.
func (h *{{M}}Handler) Routes(mux *http.ServeMux) {
`))
	if has(apiquery.OpList) {
		b.WriteString(r.Replace("\tmux.HandleFunc(\"GET {{base}}/{{table}}\", h.list)\n"))
	}
	if has(apiquery.OpCreate) {
		b.WriteString(r.Replace("\tmux.HandleFunc(\"POST {{base}}/{{table}}\", h.create)\n"))
	}
	if has(apiquery.OpGet) {
		b.WriteString(r.Replace("\tmux.HandleFunc(\"GET {{base}}/{{table}}/{id}\", h.get)\n"))
	}
	if has(apiquery.OpUpdate) {
		b.WriteString(r.Replace("\tmux.HandleFunc(\"PUT {{base}}/{{table}}/{id}\", h.update)\n"))
	}
	if has(apiquery.OpDelete) {
		b.WriteString(r.Replace("\tmux.HandleFunc(\"DELETE {{base}}/{{table}}/{id}\", h.delete)\n"))
	}
	b.WriteString("}\n")

	b.WriteString(r.Replace(`
// dto{{M}} is the JSON form of {{models}}.{{M}}: one key per column.
type dto{{M}} struct {
`))
	for _, f := range hm.fields {
		b.WriteString("\t" + f.f.GoName + " " + f.f.GoType + " `json:\"" + f.f.Column + "\"`\n")
	}
	b.WriteString("}\n")

	b.WriteString(r.Replace(`
func dto{{M}}From(m *{{models}}.{{M}}) dto{{M}} {
	d := dto{{M}}{
`))
	for _, f := range hm.fields {
		b.WriteString("\t\t" + f.f.GoName + ": m." + f.f.GoName + ",\n")
	}
	b.WriteString("\t}\n")
	for _, f := range hm.fields {
		if f.emptyVec {
			// The document says the property is an array, never null.
			b.WriteString("\tif d." + f.f.GoName + " == nil {\n\t\td." + f.f.GoName + " = vector.Vector{}\n\t}\n")
		}
	}
	b.WriteString("\treturn d\n}\n")

	if write {
		var required []string
		var notNull []string
		known := ""
		for _, f := range hm.fields {
			known += "\t\"" + f.f.Column + "\": true,\n"
			if f.required {
				required = append(required, `"`+f.f.Column+`"`)
			}
			if !f.f.Nullable && !strings.HasPrefix(f.f.GoType, "*") && !f.emptyVec {
				notNull = append(notNull, "\t\""+f.f.Column+"\": true,\n")
			}
		}
		b.WriteString(r.Replace(`
// {{lm}}Columns are the properties of a {{M}}, by their exact names.
var {{lm}}Columns = map[string]bool{
` + known + `}

// {{lm}}Required lists the properties a write must carry: not null, not filled
// in by the database.
var {{lm}}Required = []string{` + strings.Join(required, ", ") + `}

var {{lm}}NotNull = map[string]bool{
` + strings.Join(notNull, "") + `}

// apply copies the properties of d that keys names onto m. The key, identity and
// read-only columns are never written from a request.
func (d dto{{M}}) apply(m *{{models}}.{{M}}, keys map[string]json.RawMessage) {
`))
		for _, f := range hm.fields {
			if !f.settable {
				continue
			}
			b.WriteString("\tif _, ok := keys[\"" + f.f.Column + "\"]; ok {\n\t\tm." + f.f.GoName + " = d." + f.f.GoName + "\n\t}\n")
		}
		b.WriteString("}\n")
	}

	if has(apiquery.OpList) {
		b.WriteString(r.Replace(`
func (h *{{M}}Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 0
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			badRequest(w, "limit must be an integer")
			return
		}
		limit = n
	}
	limit = apiquery.ClampLimit(limit)
	sel := momo.Select[{{models}}.{{M}}](h.DB).OrderBy({{desc}}.{{PKI}}.Asc()).Limit(int64(limit + 1))
	if s := q.Get("after"); s != "" {
		c, err := apiquery.DecodeCursor(s)
		if err != nil {
			writeError(w, err)
			return
		}
		if len(c.Keys) != 1 {
			badRequest(w, "invalid page cursor")
			return
		}
		after, err := strconv.ParseInt(c.Keys[0], 10, 64)
		if err != nil {
			badRequest(w, "invalid page cursor")
			return
		}
		sel = sel.After(after)
	}
	names := make([]string, 0, len(q))
	for name := range q {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "limit" || name == "after" {
			continue
		}
		col, ok := strings.CutPrefix(name, "filter[")
		if ok {
			col, ok = strings.CutSuffix(col, "]")
		}
		if !ok {
			badRequest(w, "unknown query parameter")
			return
		}
		for _, v := range q[name] {
			p, err := apiquery.ParseFilter({{desc}}.ColumnByName, apiquery.Filter{Column: col, Op: "eq", Value: v})
			if err != nil {
				writeError(w, err)
				return
			}
			sel = sel.Where(p)
		}
	}
	rows, err := sel.All(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		c := apiquery.EncodeCursor(strconv.FormatInt(rows[limit-1].{{PK}}, 10))
		next = &c
	}
	items := make([]dto{{M}}, len(rows))
	for i := range rows {
		items[i] = dto{{M}}From(&rows[i])
	}
	writeJSON(w, http.StatusOK, struct {
		Items      []dto{{M}} ` + "`json:\"items\"`" + `
		NextCursor *string ` + "`json:\"next_cursor\"`" + `
	}{items, next})
}
`))
	}

	if has(apiquery.OpGet) {
		b.WriteString(r.Replace(`
func (h *{{M}}Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := momo.Select[{{models}}.{{M}}](h.DB).Where({{desc}}.{{PKI}}.Eq(id)).One(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto{{M}}From(&m))
}
`))
	}

	if has(apiquery.OpCreate) {
		b.WriteString(r.Replace(`
func (h *{{M}}Handler) create(w http.ResponseWriter, r *http.Request) {
	keys, data, ok := readBody(w, r)
	if !ok || !checkKeys(w, keys, {{lm}}Columns, {{lm}}Required, {{lm}}NotNull) {
		return
	}
	var in dto{{M}}
	if !decodeDTO(w, data, &in) {
		return
	}
	var m {{models}}.{{M}}
	in.apply(&m, keys)
	ins := momo.Insert[{{models}}.{{M}}](h.DB).Model(&m).ReturningAll()
`))
		for _, f := range hm.fields {
			if f.settable && f.defaulted {
				b.WriteString(r.Replace("\tif _, ok := keys[\"" + f.f.Column + "\"]; !ok {\n\t\tins = ins.Omit({{desc}}." + f.ident + ")\n\t}\n"))
			}
			if f.emptyVec {
				b.WriteString(r.Replace("\tif len(m." + f.f.GoName + ") == 0 {\n\t\tins = ins.Omit({{desc}}." + f.ident + ")\n\t}\n"))
			}
		}
		b.WriteString(r.Replace(`	rows, err := ins.Rows(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	for rows.Next() {
		if err := rows.ScanModel(&m); err != nil {
			_ = rows.Close()
			writeError(w, err)
			return
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		writeError(w, err)
		return
	}
	if err := rows.Close(); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto{{M}}From(&m))
}
`))
	}

	if has(apiquery.OpUpdate) {
		b.WriteString(r.Replace(`
func (h *{{M}}Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	keys, data, ok := readBody(w, r)
	if !ok || !checkKeys(w, keys, {{lm}}Columns, {{lm}}Required, {{lm}}NotNull) {
		return
	}
	var in dto{{M}}
	if !decodeDTO(w, data, &in) {
		return
	}
	ctx := r.Context()
	// Only the properties the request carried are written, in one statement:
	// reading the row, merging and writing every column back would undo a
	// concurrent change of a column the request did not mention.
	var sets []momo.Assignment
`))
		for _, f := range hm.fields {
			if !f.updatable {
				continue
			}
			var set string
			switch {
			case f.emptyVec:
				set = "if len(in." + f.f.GoName + ") == 0 {\n\t\t\tsets = append(sets, {{desc}}." + f.ident + ".SetDefault())\n\t\t} else {\n\t\t\tsets = append(sets, {{desc}}." + f.ident + ".Set(in." + f.f.GoName + "))\n\t\t}"
			case strings.HasPrefix(f.f.GoType, "*"):
				set = "if in." + f.f.GoName + " == nil {\n\t\t\tsets = append(sets, {{desc}}." + f.ident + ".SetNull())\n\t\t} else {\n\t\t\tsets = append(sets, {{desc}}." + f.ident + ".Set(*in." + f.f.GoName + "))\n\t\t}"
			default:
				set = "sets = append(sets, {{desc}}." + f.ident + ".Set(in." + f.f.GoName + "))"
			}
			b.WriteString(r.Replace("\tif _, ok := keys[\"" + f.f.Column + "\"]; ok {\n\t\t" + set + "\n\t}\n"))
		}
		b.WriteString(r.Replace(`	if len(sets) > 0 {
		if _, err := momo.Update[{{models}}.{{M}}](h.DB).Where({{desc}}.{{PKI}}.Eq(id)).Set(sets...).Exec(ctx); err != nil {
			writeError(w, err)
			return
		}
	}
	out, err := momo.Select[{{models}}.{{M}}](h.DB).Where({{desc}}.{{PKI}}.Eq(id)).One(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto{{M}}From(&out))
}
`))
	}

	if has(apiquery.OpDelete) {
		b.WriteString(r.Replace(`
func (h *{{M}}Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := momo.Delete[{{models}}.{{M}}](h.DB).Where({{desc}}.{{PKI}}.Eq(id)).Exec(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if res.RowsAffected == 0 {
		writeError(w, momo.ErrNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
`))
	}
}
