// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/internal/gosrc"
)

var goPkgNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// pbScalarGo is the Go type protoc-gen-go uses for a proto scalar.
var pbScalarGo = map[string]string{
	"int64": "int64", "int32": "int32", "uint32": "uint32", "uint64": "uint64",
	"string": "string", "bool": "bool", "double": "float64", "float": "float32", "bytes": "[]byte",
}

// convertible lists the Go integer types the server converts to and from the
// proto integer of a column, by an explicit conversion.
var convertible = map[string]bool{
	"int": true, "int8": true, "int16": true, "uint8": true, "uint16": true, "uint": true,
}

// pbReserved are the names protoc-gen-go does not hand to a field: it appends
// an underscore to them, so the generated field would not be called by them.
var pbReserved = map[string]bool{
	"Reset": true, "String": true, "ProtoMessage": true, "ProtoReflect": true, "Descriptor": true,
}

// goCamel is protoc-gen-go's GoCamelCase: the Go name of a proto field.
func goCamel(s string) string {
	isLower := func(c byte) bool { return 'a' <= c && c <= 'z' }
	isDigit := func(c byte) bool { return '0' <= c && c <= '9' }
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_' && (i == 0 || s[i-1] == '.'):
			b = append(b, 'X')
		case c == '_' && i+1 < len(s) && isLower(s[i+1]):
			// "_" before a lower-case letter is dropped; the letter is upper-cased.
		case isDigit(c):
			b = append(b, c)
		default:
			if isLower(c) {
				c -= 'a' - 'A'
			}
			b = append(b, c)
			for ; i+1 < len(s) && isLower(s[i+1]); i++ {
				b = append(b, s[i+1])
			}
		}
	}
	return string(b)
}

// sfield is one exposed column as the server maps it between the model and
// the message.
type sfield struct {
	goName string // model field
	pbName string // message field
	// lit is the "Name: expr," entry of the message literal, or "" when the
	// field is set by stmt after the literal.
	lit string
	// stmt sets the message field after the literal ("" for none).
	stmt string
	// from copies the message field onto the model m from the message p.
	from string
	// emptyVec marks a vector the database cannot store when empty.
	emptyVec bool
}

// serverField maps one exposed column; msg is non-empty when it cannot.
func serverField(m *gen.Model, f *gen.Field, pf protoField) (sf sfield, msg string) {
	n, p := f.GoName, goCamel(f.Column)
	sf = sfield{goName: n, pbName: p}
	switch {
	case len(f.Path) != 1:
		return sf, "the gRPC server does not support the embedded field " + n + " of " + m.GoName
	case pbReserved[p]:
		return sf, "the gRPC server cannot map " + m.GoName + "." + n + ": protoc-gen-go renames the field " + p
	}
	base := strings.TrimPrefix(f.GoType, "*")
	ptr := strings.HasPrefix(f.GoType, "*")
	unsupported := "the gRPC server does not support the type " + f.GoType + " of " + m.GoName + "." + n
	switch {
	case pf.typ == "google.protobuf.Timestamp":
		if base != "time.Time" {
			return sf, unsupported
		}
		if ptr {
			sf.stmt = "if m." + n + " != nil {\n\t\tout." + p + " = timestamppb.New(*m." + n + ")\n\t}"
			sf.from = "if p." + p + " != nil {\n\t\tt := p." + p + ".AsTime()\n\t\tm." + n + " = &t\n\t} else {\n\t\tm." + n + " = nil\n\t}"
			return sf, ""
		}
		sf.lit = p + ": timestamppb.New(m." + n + "),"
		sf.from = "if p." + p + " != nil {\n\t\tm." + n + " = p." + p + ".AsTime()\n\t}"
		return sf, ""
	case pf.repeated:
		if f.GoType != "vector.Vector" {
			return sf, unsupported
		}
		sf.lit = p + ": []float32(m." + n + "),"
		sf.from = "if len(p." + p + ") > 0 {\n\t\tm." + n + " = vector.Vector(p." + p + ")\n\t}"
		sf.emptyVec = true
		return sf, ""
	}
	pbGo, ok := pbScalarGo[pf.typ]
	switch {
	case !ok:
		return sf, unsupported
	case base == pbGo && ptr == pf.optional:
		sf.lit = p + ": m." + n + ","
		sf.from = "m." + n + " = p." + p
		return sf, ""
	case !ptr && !pf.optional && convertible[base]:
		sf.lit = p + ": " + pbGo + "(m." + n + "),"
		sf.from = "m." + n + " = " + base + "(p." + p + ")"
		return sf, ""
	}
	return sf, unsupported
}

type smodel struct {
	m          *gen.Model
	fields     []sfield
	pk         sfield
	modelAlias string
	descAlias  string
}

// ServerSource returns the Go source of the gRPC server adapters for the
// models of g, in package pkg, and the diagnostics for columns the adapters
// cannot carry. pbImport is the import path of the Go package protoc-gen-go
// generates from the .proto file Proto writes for the same lock. Each model
// with a single int64 primary key gets New<Model>Server(db *momo.DB), which
// returns the generated <Model>ServiceServer. The adapters use the typed
// builders only; List filters through the generated ColumnByName allowlist,
// pages with keyset cursors on the primary key and clamps the limit. An
// update reads the row first, so a column that is not in the message keeps its
// value. The output is deterministic.
func ServerSource(g *gen.Graph, lock *Lock, pkg, pbImport string) ([]byte, []gen.Diagnostic) {
	if !goPkgNameRE.MatchString(pkg) {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid Go package name %q for the gRPC server", pkg)}}
	}
	if !goPackageRE.MatchString(pbImport) || strings.Contains(pbImport, ";") {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid import path %q for the generated protobuf package", pbImport)}}
	}
	imports := map[string]string{
		"context": "context", "errors": "errors", "strconv": "strconv",
		"momo":        "github.com/azrtydxb/momo",
		"vector":      "github.com/azrtydxb/momo/vector",
		"apiquery":    "github.com/azrtydxb/momo/gen/apiquery",
		"codes":       "google.golang.org/grpc/codes",
		"status":      "google.golang.org/grpc/status",
		"timestamppb": "google.golang.org/protobuf/types/known/timestamppb",
		"pb":          pbImport,
	}
	aliasOf := map[string]string{}
	taken := map[string]bool{}
	for a := range imports {
		taken[a] = true
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

	var diags []gen.Diagnostic
	var models []smodel
	taken2 := map[string]string{}
	for _, m := range gosrc.SortedModels(g) {
		if !identRE.MatchString(m.GoName) {
			continue // Proto reports it
		}
		clash := false
		for _, n := range derivedMessages(m.GoName) {
			if _, dup := taken2[n]; dup || n == "Filter" {
				clash = true
			}
		}
		if clash {
			continue // Proto reports it
		}
		for _, n := range derivedMessages(m.GoName) {
			taken2[n] = m.GoName
		}
		pfs, _, _, _ := fieldsOf(m, lock)
		byCol := map[string]*gen.Field{}
		for _, f := range m.Fields {
			if _, dup := byCol[f.Column]; !dup {
				byCol[f.Column] = f
			}
		}
		sm := smodel{m: m}
		bad := false
		pk := gosrc.SinglePK(g, m)
		havePK := false
		for _, pf := range pfs {
			f := byCol[pf.column]
			sf, msg := serverField(m, f, pf)
			if msg != "" {
				diags = append(diags, gen.Diagnostic{Pos: f.Pos, Msg: msg})
				bad = true
				continue
			}
			sm.fields = append(sm.fields, sf)
			if f == pk {
				sm.pk, havePK = sf, true
			}
		}
		if bad || pk == nil || pk.GoType != "int64" || !havePK {
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
		sm.modelAlias = alias(mp, m.PkgName)
		sm.descAlias = alias(dp, m.SubPkg+"desc")
		models = append(models, sm)
	}
	if len(diags) > 0 {
		return nil, diags
	}

	var b strings.Builder
	b.WriteString(serverPreamble)
	for _, sm := range models {
		writeServer(&b, sm)
	}
	body := b.String()

	var out strings.Builder
	out.WriteString(gen.Header() + "\n\n")
	out.WriteString("// Package " + pkg + " holds gRPC server adapters for the models, generated from the\n// same graph as the .proto file.\npackage " + pkg + "\n\n")
	out.WriteString(gosrc.Imports(body, imports))
	out.WriteString(body)
	return []byte(out.String()), nil
}

const serverPreamble = `// statusOf maps a failed statement to a gRPC status: not found is NotFound, a
// rejected value is InvalidArgument and anything else is Internal without a
// detail.
func statusOf(err error) error {
	switch {
	case errors.Is(err, momo.ErrNotFound):
		return status.Error(codes.NotFound, "not found")
	case errors.Is(err, momo.ErrValidation):
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Error(codes.Internal, "internal error")
}
`

func writeServer(b *strings.Builder, sm smodel) {
	m := sm.m
	lower := gosrc.LowerFirst(m.GoName)
	r := strings.NewReplacer(
		"{{M}}", m.GoName,
		"{{lm}}", lower,
		"{{models}}", sm.modelAlias,
		"{{desc}}", sm.descAlias,
		"{{PK}}", sm.pk.goName,
		"{{PBPK}}", sm.pk.pbName,
	)
	b.WriteString(r.Replace(`
// {{lm}}Server implements pb.{{M}}ServiceServer on top of momo.
type {{lm}}Server struct {
	pb.Unimplemented{{M}}ServiceServer
	db *momo.DB
}

// New{{M}}Server returns the {{M}}ServiceServer backed by db.
func New{{M}}Server(db *momo.DB) pb.{{M}}ServiceServer {
	return &{{lm}}Server{db: db}
}

func {{lm}}ToPB(m *{{models}}.{{M}}) *pb.{{M}} {
	out := &pb.{{M}}{
`))
	for _, f := range sm.fields {
		if f.lit != "" {
			b.WriteString("\t\t" + f.lit + "\n")
		}
	}
	b.WriteString("\t}\n")
	for _, f := range sm.fields {
		if f.stmt != "" {
			b.WriteString("\t" + f.stmt + "\n")
		}
	}
	b.WriteString(r.Replace(`	return out
}

// {{lm}}FromPB copies the message onto m. An empty vector leaves the vector of m alone.
func {{lm}}FromPB(p *pb.{{M}}, m *{{models}}.{{M}}) {
`))
	for _, f := range sm.fields {
		b.WriteString("\t" + f.from + "\n")
	}
	b.WriteString("}\n")

	b.WriteString(r.Replace(`
func (s *{{lm}}Server) Get(ctx context.Context, req *pb.{{M}}GetRequest) (*pb.{{M}}, error) {
	m, err := momo.Select[{{models}}.{{M}}](s.db).Where({{desc}}.{{PK}}.Eq(req.Get{{PBPK}}())).One(ctx)
	if err != nil {
		return nil, statusOf(err)
	}
	return {{lm}}ToPB(&m), nil
}

func (s *{{lm}}Server) List(ctx context.Context, req *pb.{{M}}ListRequest) (*pb.{{M}}ListResponse, error) {
	limit := apiquery.ClampLimit(int(req.GetLimit()))
	sel := momo.Select[{{models}}.{{M}}](s.db).OrderBy({{desc}}.{{PK}}.Asc()).Limit(int64(limit + 1))
	if after := req.GetAfter(); after != "" {
		c, err := apiquery.DecodeCursor(after)
		if err != nil {
			return nil, statusOf(err)
		}
		if len(c.Keys) != 1 {
			return nil, status.Error(codes.InvalidArgument, "invalid page cursor")
		}
		id, err := strconv.ParseInt(c.Keys[0], 10, 64)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid page cursor")
		}
		sel = sel.After(id)
	}
	for _, f := range req.GetFilter() {
		p, err := apiquery.ParseFilter({{desc}}.ColumnByName, apiquery.Filter{Column: f.GetColumn(), Op: f.GetOp(), Value: f.GetValue()})
		if err != nil {
			return nil, statusOf(err)
		}
		sel = sel.Where(p)
	}
	rows, err := sel.All(ctx)
	if err != nil {
		return nil, statusOf(err)
	}
	resp := &pb.{{M}}ListResponse{}
	if len(rows) > limit {
		rows = rows[:limit]
		resp.NextCursor = apiquery.EncodeCursor(strconv.FormatInt(rows[limit-1].{{PK}}, 10))
	}
	for i := range rows {
		resp.Items = append(resp.Items, {{lm}}ToPB(&rows[i]))
	}
	return resp, nil
}

func (s *{{lm}}Server) Create(ctx context.Context, in *pb.{{M}}) (*pb.{{M}}, error) {
	var m {{models}}.{{M}}
	{{lm}}FromPB(in, &m)
	ins := momo.Insert[{{models}}.{{M}}](s.db).Model(&m).ReturningAll()
`))
	for _, f := range sm.fields {
		if f.emptyVec {
			b.WriteString(r.Replace("\tif len(m." + f.goName + ") == 0 {\n\t\tins = ins.Omit({{desc}}." + f.goName + ")\n\t}\n"))
		}
	}
	b.WriteString(r.Replace(`	rows, err := ins.Rows(ctx)
	if err != nil {
		return nil, statusOf(err)
	}
	for rows.Next() {
		if err := rows.ScanModel(&m); err != nil {
			_ = rows.Close()
			return nil, statusOf(err)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, statusOf(err)
	}
	if err := rows.Close(); err != nil {
		return nil, statusOf(err)
	}
	return {{lm}}ToPB(&m), nil
}

func (s *{{lm}}Server) Update(ctx context.Context, in *pb.{{M}}) (*pb.{{M}}, error) {
	id := in.Get{{PBPK}}()
	m, err := momo.Select[{{models}}.{{M}}](s.db).Where({{desc}}.{{PK}}.Eq(id)).One(ctx)
	if err != nil {
		return nil, statusOf(err)
	}
	{{lm}}FromPB(in, &m)
	m.{{PK}} = id
	upd := momo.Update[{{models}}.{{M}}](s.db).Model(&m)
`))
	for _, f := range sm.fields {
		if f.emptyVec {
			b.WriteString(r.Replace("\tif len(m." + f.goName + ") == 0 {\n\t\tupd = upd.Set({{desc}}." + f.goName + ".SetDefault())\n\t}\n"))
		}
	}
	b.WriteString(r.Replace(`	if _, err := upd.Exec(ctx); err != nil {
		return nil, statusOf(err)
	}
	out, err := momo.Select[{{models}}.{{M}}](s.db).Where({{desc}}.{{PK}}.Eq(id)).One(ctx)
	if err != nil {
		return nil, statusOf(err)
	}
	return {{lm}}ToPB(&out), nil
}

func (s *{{lm}}Server) Delete(ctx context.Context, req *pb.{{M}}DeleteRequest) (*pb.{{M}}DeleteResponse, error) {
	res, err := momo.Delete[{{models}}.{{M}}](s.db).Where({{desc}}.{{PK}}.Eq(req.Get{{PBPK}}())).Exec(ctx)
	if err != nil {
		return nil, statusOf(err)
	}
	if res.RowsAffected == 0 {
		return nil, statusOf(momo.ErrNotFound)
	}
	return &pb.{{M}}DeleteResponse{}, nil
}
`))
}
