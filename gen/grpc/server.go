// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"fmt"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/apiquery"
	"github.com/azrtydxb/momo/gen/internal/gosrc"
)

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
	// ident is the name of the column's descriptor in the descriptor package.
	ident string
	// settable is false for the key, identity and read-only columns, which a
	// request never writes.
	settable bool
	// defaulted marks a column the database fills in; a scalar left at its zero
	// value is omitted on create and keeps its stored value on update.
	defaulted bool
	// scalar marks a field isZero can test (not a vector or a timestamp).
	scalar bool
	// updatable is set for a column an Update can assign: writable, and none of
	// the version, soft-delete and timestamp columns momo owns.
	updatable bool
	// kind says how an Update assigns the column: scalar, optional (a pointer
	// scalar), vector, time or timeptr.
	kind string
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
			sf.kind = "timeptr"
			return sf, ""
		}
		sf.lit = p + ": timestamppb.New(m." + n + "),"
		sf.from = "if p." + p + " != nil {\n\t\tm." + n + " = p." + p + ".AsTime()\n\t}"
		sf.kind = "time"
		return sf, ""
	case pf.repeated:
		if f.GoType != "vector.Vector" {
			return sf, unsupported
		}
		sf.lit = p + ": []float32(m." + n + "),"
		sf.from = "if len(p." + p + ") > 0 {\n\t\tm." + n + " = vector.Vector(p." + p + ")\n\t}"
		sf.emptyVec = true
		sf.kind = "vector"
		return sf, ""
	}
	pbGo, ok := pbScalarGo[pf.typ]
	switch {
	case !ok:
		return sf, unsupported
	case base == pbGo && ptr == pf.optional:
		sf.lit = p + ": m." + n + ","
		sf.from = "m." + n + " = p." + p
		sf.scalar = true
		sf.kind = "scalar"
		if ptr {
			sf.kind = "optional"
		}
		return sf, ""
	case !ptr && !pf.optional && convertible[base]:
		sf.lit = p + ": " + pbGo + "(m." + n + "),"
		sf.from = "m." + n + " = " + base + "(p." + p + ")"
		sf.scalar = true
		sf.kind = "scalar"
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
	ops        apiquery.OpSet
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
	return serverSource(g, lock, pkg, pbImport, nil)
}

// serverSource is ServerSource with the operations ops enables per model.
func serverSource(g *gen.Graph, lock *Lock, pkg, pbImport string, ops apiquery.Ops) ([]byte, []gen.Diagnostic) {
	if err := ops.Validate(); err != nil {
		return nil, []gen.Diagnostic{{Msg: "gRPC " + err.Error()}}
	}
	if !gosrc.PkgName.MatchString(pkg) {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid Go package name %q for the gRPC server", pkg)}}
	}
	if !goPackageRE.MatchString(pbImport) || strings.Contains(pbImport, ";") {
		return nil, []gen.Diagnostic{{Msg: fmt.Sprintf("invalid import path %q for the generated protobuf package", pbImport)}}
	}
	aliases := gosrc.NewAliases(map[string]string{
		"context": "context", "errors": "errors", "strconv": "strconv",
		"momo":        "github.com/azrtydxb/momo",
		"vector":      "github.com/azrtydxb/momo/vector",
		"apiquery":    "github.com/azrtydxb/momo/gen/apiquery",
		"codes":       "google.golang.org/grpc/codes",
		"status":      "google.golang.org/grpc/status",
		"timestamppb": "google.golang.org/protobuf/types/known/timestamppb",
		"pb":          pbImport,
	})
	alias := aliases.Alias

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
		idents, _ := gen.DescriptorIdents(m, g)
		byCol := map[string]*gen.Field{}
		identOf := map[*gen.Field]string{}
		for i, f := range m.Fields {
			identOf[f] = idents[i]
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
			sf.ident = identOf[f]
			sf.settable = f != pk && !f.Tags.Identity && !f.Tags.Serial && f.Access != gen.ReadOnly
			sf.defaulted = f.Tags.Default != ""
			sf.updatable = sf.settable && f.Access == gen.Writable && !f.Tags.Version && f.Tags.SoftDelete == "" && !f.Tags.CreatedAt && !f.Tags.UpdatedAt
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
		sm.ops = ops.For(m.GoName)
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
	out.WriteString(gosrc.Imports(body, aliases.Candidates()))
	out.WriteString(body)
	return []byte(out.String()), nil
}

const serverPreamble = `// statusOf maps a failed statement to a gRPC status with a fixed message: the
// error of the database, which names tables and constraints, never reaches the
// client.
func statusOf(err error) error {
	switch {
	case errors.Is(err, momo.ErrNotFound):
		return status.Error(codes.NotFound, "not found")
	case errors.Is(err, momo.ErrUniqueViolation):
		return status.Error(codes.AlreadyExists, "a unique constraint refused the change")
	case errors.Is(err, momo.ErrForeignKeyViolation):
		return status.Error(codes.FailedPrecondition, "a foreign key constraint refused the change")
	case errors.Is(err, momo.ErrCheckViolation), errors.Is(err, momo.ErrNotNullViolation):
		return status.Error(codes.InvalidArgument, "a constraint refused the value")
	case errors.Is(err, momo.ErrBusy), errors.Is(err, momo.ErrSerialization):
		return status.Error(codes.Unavailable, "temporarily unavailable; retry")
	case errors.Is(err, momo.ErrValidation):
		return status.Error(codes.InvalidArgument, "invalid request")
	}
	return status.Error(codes.Internal, "internal error")
}

// isZero reports whether v is the zero value of its type.
func isZero[T comparable](v T) bool {
	var zero T
	return v == zero
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
		"{{PKI}}", sm.pk.ident,
		"{{PBPK}}", sm.pk.pbName,
	)
	has := sm.ops.Has
	b.WriteString(r.Replace(`
// {{lm}}Server implements pb.{{M}}ServiceServer on top of momo. It does no
// authentication or authorization: wrap the grpc.Server with interceptors.
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
	b.WriteString("\treturn out\n}\n")

	if has(apiquery.OpCreate) || has(apiquery.OpUpdate) {
		b.WriteString(r.Replace(`
// {{lm}}FromPB copies the message onto m. The key, identity and read-only
// columns are never written. A vector or timestamp left out, and a scalar of a
// column the database fills in left at its zero value, leave m alone.
func {{lm}}FromPB(p *pb.{{M}}, m *{{models}}.{{M}}) {
`))
		for _, f := range sm.fields {
			if !f.settable {
				continue
			}
			from := f.from
			if f.defaulted && f.scalar {
				from = "if !isZero(p." + f.pbName + ") {\n\t\t" + f.from + "\n\t}"
			}
			b.WriteString("\t" + from + "\n")
		}
		b.WriteString("}\n")
	}

	if has(apiquery.OpGet) {
		b.WriteString(r.Replace(`
func (s *{{lm}}Server) Get(ctx context.Context, req *pb.{{M}}GetRequest) (*pb.{{M}}, error) {
	m, err := momo.Select[{{models}}.{{M}}](s.db).Where({{desc}}.{{PKI}}.Eq(req.Get{{PBPK}}())).One(ctx)
	if err != nil {
		return nil, statusOf(err)
	}
	return {{lm}}ToPB(&m), nil
}
`))
	}

	if has(apiquery.OpList) {
		b.WriteString(r.Replace(`
func (s *{{lm}}Server) List(ctx context.Context, req *pb.{{M}}ListRequest) (*pb.{{M}}ListResponse, error) {
	limit := apiquery.ClampLimit(int(req.GetLimit()))
	sel := momo.Select[{{models}}.{{M}}](s.db).OrderBy({{desc}}.{{PKI}}.Asc()).Limit(int64(limit + 1))
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
`))
	}

	if has(apiquery.OpCreate) {
		b.WriteString(r.Replace(`
func (s *{{lm}}Server) Create(ctx context.Context, in *pb.{{M}}) (*pb.{{M}}, error) {
	var m {{models}}.{{M}}
	{{lm}}FromPB(in, &m)
	ins := momo.Insert[{{models}}.{{M}}](s.db).Model(&m).ReturningAll()
`))
		for _, f := range sm.fields {
			switch {
			case f.emptyVec:
				b.WriteString(r.Replace("\tif len(m." + f.goName + ") == 0 {\n\t\tins = ins.Omit({{desc}}." + f.ident + ")\n\t}\n"))
			case f.settable && f.defaulted && f.scalar:
				b.WriteString(r.Replace("\tif isZero(m." + f.goName + ") {\n\t\tins = ins.Omit({{desc}}." + f.ident + ")\n\t}\n"))
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
`))
	}

	if has(apiquery.OpUpdate) {
		b.WriteString(r.Replace(`
// Update replaces the row: every writable column takes the value of the
// message, except the vectors, timestamps and defaulted scalars it leaves out
// (a proto3 message cannot say a scalar is absent, so a scalar at its zero value
// is a value, and so is an unset optional field, which stores NULL). It writes
// the assignments in one statement and reads nothing first, so it never writes
// back a value it read: a concurrent change of a column the message leaves out
// is kept.
func (s *{{lm}}Server) Update(ctx context.Context, in *pb.{{M}}) (*pb.{{M}}, error) {
	id := in.Get{{PBPK}}()
	var m {{models}}.{{M}}
	{{lm}}FromPB(in, &m)
	var sets []momo.Assignment
`))
		for _, f := range sm.fields {
			if !f.updatable {
				continue
			}
			set := "sets = append(sets, {{desc}}." + f.ident + ".Set(m." + f.goName + "))"
			null := "if m." + f.goName + " == nil {\n\t\tsets = append(sets, {{desc}}." + f.ident + ".SetNull())\n\t} else {\n\t\tsets = append(sets, {{desc}}." + f.ident + ".Set(*m." + f.goName + "))\n\t}"
			var code string
			switch f.kind {
			case "scalar":
				code = set
				if f.defaulted {
					code = "if !isZero(in." + "Get" + f.pbName + "()) {\n\t\t" + set + "\n\t}"
				}
			case "optional", "timeptr":
				code = null
			case "vector":
				code = "if len(in.Get" + f.pbName + "()) > 0 {\n\t\t" + set + "\n\t}"
			case "time":
				code = "if in.Get" + f.pbName + "() != nil {\n\t\t" + set + "\n\t}"
			default:
				continue
			}
			b.WriteString(r.Replace("\t" + code + "\n"))
		}
		b.WriteString(r.Replace(`	if len(sets) > 0 {
		if _, err := momo.Update[{{models}}.{{M}}](s.db).Where({{desc}}.{{PKI}}.Eq(id)).Set(sets...).Exec(ctx); err != nil {
			return nil, statusOf(err)
		}
	}
	out, err := momo.Select[{{models}}.{{M}}](s.db).Where({{desc}}.{{PKI}}.Eq(id)).One(ctx)
	if err != nil {
		return nil, statusOf(err)
	}
	return {{lm}}ToPB(&out), nil
}
`))
	}

	if has(apiquery.OpDelete) {
		b.WriteString(r.Replace(`
func (s *{{lm}}Server) Delete(ctx context.Context, req *pb.{{M}}DeleteRequest) (*pb.{{M}}DeleteResponse, error) {
	res, err := momo.Delete[{{models}}.{{M}}](s.db).Where({{desc}}.{{PKI}}.Eq(req.Get{{PBPK}}())).Exec(ctx)
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
}
