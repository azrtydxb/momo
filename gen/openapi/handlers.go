// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/internal/gosrc"
)

var (
	pkgNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	jsonKeyRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

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
	f *gen.Field
	// emptyVec is set for a non-pointer vector, which the database cannot
	// store when empty: the handlers leave such a column out of an insert and
	// reset it on an update.
	emptyVec bool
}

type hmodel struct {
	m          *gen.Model
	modelAlias string
	descAlias  string
	fields     []hfield
	pk         *gen.Field
}

// handlerFields returns the exposed columns of m, the same set the document
// describes, and a diagnostic for a column the handlers cannot carry.
func handlerFields(m *gen.Model) ([]hfield, []gen.Diagnostic) {
	var out []hfield
	var diags []gen.Diagnostic
	seen := map[string]bool{}
	for _, f := range m.Fields {
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
			out = append(out, hfield{f: f, emptyVec: f.GoType == "vector.Vector"})
		}
	}
	return out, diags
}

// Handlers returns the Go source of net/http handlers for the models of g, in
// package pkg, and the diagnostics for columns the handlers cannot carry.
// Each model with a single int64 primary key gets a type MHandler with a DB
// field and a Routes method that registers the five routes of the document
// on a *http.ServeMux; a model without such a key has no handlers. The
// handlers use the typed builders only, filter through the generated
// ColumnByName allowlist, page with keyset cursors on the primary key, limit
// request bodies to 1 MiB and answer errors as {"error": "..."}. The
// response keys are exactly the column properties of the model's schema. The
// output is deterministic.
func Handlers(g *gen.Graph, pkg string, opts Options) ([]byte, []gen.Diagnostic) {
	var diags []gen.Diagnostic
	if !pkgNameRE.MatchString(pkg) {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid Go package name %q for the OpenAPI handlers", pkg)}}
	}
	base := strings.TrimRight(opts.BasePath, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("OpenAPI BasePath %q must be empty or start with a slash", opts.BasePath)}}
	}

	imports := map[string]string{
		"json": "encoding/json", "errors": "errors", "http": "net/http", "sort": "sort",
		"strconv": "strconv", "strings": "strings", "time": "time",
		"momo": "github.com/azrtydxb/momo", "vector": "github.com/azrtydxb/momo/vector",
		"apiquery": "github.com/azrtydxb/momo/gen/apiquery",
	}
	aliasOf := map[string]string{} // import path -> alias
	taken := map[string]bool{}
	for alias := range imports {
		taken[alias] = true
	}
	alias := func(path, want string) string {
		if a, ok := aliasOf[path]; ok {
			return a
		}
		a := want
		for n := 2; taken[a]; n++ {
			a = want + strconv.Itoa(n)
		}
		taken[a] = true
		aliasOf[path] = a
		imports[a] = path
		return a
	}

	var models []hmodel
	seenName := map[string]bool{}
	seenTable := map[string]bool{}
	for _, m := range gosrc.SortedModels(g) {
		if seenName[m.GoName] || m.GoName == "Error" || !schemaNameRE.MatchString(m.GoName) || !pathSegmentRE.MatchString(m.Table) || seenTable[m.Table] {
			continue // Document reports it
		}
		seenName[m.GoName] = true
		seenTable[m.Table] = true
		fields, fd := handlerFields(m)
		diags = append(diags, fd...)
		pk := gosrc.SinglePK(g, m)
		if pk == nil || pk.GoType != "int64" || pk.Tags.Sensitive {
			continue
		}
		exposed := false
		for _, f := range fields {
			exposed = exposed || f.f == pk
		}
		if !exposed {
			continue
		}
		mp, err := gosrc.ModelImport(m)
		if err != nil {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: err.Error()})
			continue
		}
		dp, err := gosrc.DescImport(m)
		if err != nil {
			diags = append(diags, gen.Diagnostic{Pos: m.Pos, Msg: err.Error()})
			continue
		}
		models = append(models, hmodel{
			m: m, fields: fields, pk: pk,
			modelAlias: alias(mp, m.PkgName),
			descAlias:  alias(dp, m.SubPkg+"desc"),
		})
	}
	if len(diags) > 0 {
		return nil, diags
	}

	var b strings.Builder
	b.WriteString(preamble)
	for _, hm := range models {
		writeHandler(&b, hm, base)
	}
	body := b.String()

	var out strings.Builder
	out.WriteString(gen.Header() + "\n\n")
	out.WriteString("// Package " + pkg + " holds net/http handlers for the models, generated from the\n// same graph as the OpenAPI document.\npackage " + pkg + "\n\n")
	out.WriteString(gosrc.Imports(body, imports))
	out.WriteString(body)
	return []byte(out.String()), nil
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

// writeError answers a failed statement: not found is 404, a rejected value
// is 400 and anything else is 500 without a detail.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, momo.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not found"})
	case errors.Is(err, momo.ErrValidation):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal error"})
	}
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, errorBody{Error: msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		badRequest(w, "invalid request body")
		return false
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
		"{{models}}", hm.modelAlias,
		"{{desc}}", hm.descAlias,
		"{{PK}}", hm.pk.GoName,
		"{{base}}", base,
		"{{table}}", m.Table,
	)

	b.WriteString(r.Replace(`
// {{M}}Handler serves the {{M}} endpoints of the OpenAPI document.
type {{M}}Handler struct{ DB *momo.DB }

// Routes registers the {{M}} routes on mux.
func (h *{{M}}Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET {{base}}/{{table}}", h.list)
	mux.HandleFunc("POST {{base}}/{{table}}", h.create)
	mux.HandleFunc("GET {{base}}/{{table}}/{id}", h.get)
	mux.HandleFunc("PUT {{base}}/{{table}}/{id}", h.update)
	mux.HandleFunc("DELETE {{base}}/{{table}}/{id}", h.delete)
}

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

	b.WriteString(r.Replace(`
// apply copies the fields of d onto m. An empty vector leaves the vector of m
// alone.
func (d dto{{M}}) apply(m *{{models}}.{{M}}) {
`))
	for _, f := range hm.fields {
		if f.emptyVec {
			b.WriteString("\tif len(d." + f.f.GoName + ") > 0 {\n\t\tm." + f.f.GoName + " = d." + f.f.GoName + "\n\t}\n")
			continue
		}
		b.WriteString("\tm." + f.f.GoName + " = d." + f.f.GoName + "\n")
	}
	b.WriteString("}\n")

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
	sel := momo.Select[{{models}}.{{M}}](h.DB).OrderBy({{desc}}.{{PK}}.Asc()).Limit(int64(limit + 1))
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

func (h *{{M}}Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := momo.Select[{{models}}.{{M}}](h.DB).Where({{desc}}.{{PK}}.Eq(id)).One(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto{{M}}From(&m))
}

func (h *{{M}}Handler) create(w http.ResponseWriter, r *http.Request) {
	var in dto{{M}}
	if !decodeBody(w, r, &in) {
		return
	}
	var m {{models}}.{{M}}
	in.apply(&m)
	ins := momo.Insert[{{models}}.{{M}}](h.DB).Model(&m).ReturningAll()
`))
	for _, f := range hm.fields {
		if f.emptyVec {
			b.WriteString(r.Replace("\tif len(m." + f.f.GoName + ") == 0 {\n\t\tins = ins.Omit({{desc}}." + f.f.GoName + ")\n\t}\n"))
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

func (h *{{M}}Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in dto{{M}}
	if !decodeBody(w, r, &in) {
		return
	}
	ctx := r.Context()
	m, err := momo.Select[{{models}}.{{M}}](h.DB).Where({{desc}}.{{PK}}.Eq(id)).One(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	in.apply(&m)
	m.{{PK}} = id
	upd := momo.Update[{{models}}.{{M}}](h.DB).Model(&m)
`))
	for _, f := range hm.fields {
		if f.emptyVec {
			b.WriteString(r.Replace("\tif len(m." + f.f.GoName + ") == 0 {\n\t\tupd = upd.Set({{desc}}." + f.f.GoName + ".SetDefault())\n\t}\n"))
		}
	}
	b.WriteString(r.Replace(`	if _, err := upd.Exec(ctx); err != nil {
		writeError(w, err)
		return
	}
	out, err := momo.Select[{{models}}.{{M}}](h.DB).Where({{desc}}.{{PK}}.Eq(id)).One(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto{{M}}From(&out))
}

func (h *{{M}}Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := momo.Delete[{{models}}.{{M}}](h.DB).Where({{desc}}.{{PK}}.Eq(id)).Exec(r.Context())
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
