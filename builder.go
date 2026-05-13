package condition

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type Builder struct {
	expr string
}

type FieldBuilder struct {
	path   string
	prefix string
}

func When(path string) FieldBuilder { return FieldBuilder{path: path} }

func Expr(raw string) Builder { return Builder{expr: strings.TrimSpace(raw)} }

func Var(name string) FieldBuilder {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "$")
	name = strings.TrimPrefix(name, ":")
	if strings.HasPrefix(name, "{{") && strings.HasSuffix(name, "}}") {
		name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(name, "{{"), "}}"))
	}
	return FieldBuilder{path: name}
}

func (f FieldBuilder) Eq(v any) Builder  { return f.cmp("==", v) }
func (f FieldBuilder) Ne(v any) Builder  { return f.cmp("!=", v) }
func (f FieldBuilder) Lt(v any) Builder  { return f.cmp("<", v) }
func (f FieldBuilder) Lte(v any) Builder { return f.cmp("<=", v) }
func (f FieldBuilder) Gt(v any) Builder  { return f.cmp(">", v) }
func (f FieldBuilder) Gte(v any) Builder { return f.cmp(">=", v) }
func (f FieldBuilder) Contains(v any) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("%s in %s", literal(v), f.path))}
}
func (f FieldBuilder) Matches(v any) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("regex_match(%s, %s)", f.path, literal(v)))}
}
func (f FieldBuilder) StartsWith(v any) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("starts_with(%s, %s)", f.path, literal(v)))}
}
func (f FieldBuilder) EndsWith(v any) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("ends_with(%s, %s)", f.path, literal(v)))}
}
func (f FieldBuilder) Exists() Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("exists(%s)", literal(f.path)))}
}
func (f FieldBuilder) Missing() Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("not exists(%s)", literal(f.path)))}
}
func (f FieldBuilder) IsNull() Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("isNull(%s)", f.path))}
}
func (f FieldBuilder) IsNotNull() Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("isNotNull(%s)", f.path))}
}
func (f FieldBuilder) Empty() Builder { return Builder{expr: f.wrap(fmt.Sprintf("empty(%s)", f.path))} }
func (f FieldBuilder) NotEmpty() Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("not empty(%s)", f.path))}
}
func (f FieldBuilder) Between(min, max any) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("between(%s, %s, %s)", f.path, literal(min), literal(max)))}
}
func (f FieldBuilder) BetweenPath(minPath, maxPath string) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("between(%s, %s, %s)", f.path, minPath, maxPath))}
}

func (f FieldBuilder) EqPath(path string) Builder  { return f.cmpPath("==", path) }
func (f FieldBuilder) NePath(path string) Builder  { return f.cmpPath("!=", path) }
func (f FieldBuilder) LtPath(path string) Builder  { return f.cmpPath("<", path) }
func (f FieldBuilder) LtePath(path string) Builder { return f.cmpPath("<=", path) }
func (f FieldBuilder) GtPath(path string) Builder  { return f.cmpPath(">", path) }
func (f FieldBuilder) GtePath(path string) Builder { return f.cmpPath(">=", path) }

func (f FieldBuilder) In(values ...any) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("%s in %s", f.path, literal(values)))}
}

func (f FieldBuilder) NotIn(values ...any) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("%s not in %s", f.path, literal(values)))}
}

func (f FieldBuilder) InPath(path string) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("%s in %s", f.path, path))}
}

func (f FieldBuilder) NotInPath(path string) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("%s not in %s", f.path, path))}
}

func (f FieldBuilder) cmp(op string, v any) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("%s %s %s", f.path, op, literal(v)))}
}

func (f FieldBuilder) cmpPath(op, path string) Builder {
	return Builder{expr: f.wrap(fmt.Sprintf("%s %s %s", f.path, op, path))}
}

func (f FieldBuilder) wrap(s string) string {
	if f.prefix == "" {
		return s
	}
	return f.prefix + " " + s
}

func (b Builder) And(path string) FieldBuilder {
	return FieldBuilder{path: path, prefix: "(" + b.expr + ") and"}
}

func (b Builder) Or(path string) FieldBuilder {
	return FieldBuilder{path: path, prefix: "(" + b.expr + ") or"}
}

func (b Builder) AndVar(name string) FieldBuilder {
	f := Var(name)
	f.prefix = "(" + b.expr + ") and"
	return f
}

func (b Builder) OrVar(name string) FieldBuilder {
	f := Var(name)
	f.prefix = "(" + b.expr + ") or"
	return f
}

func (b Builder) AndExpr(other Builder) Builder {
	return Builder{expr: fmt.Sprintf("(%s) and (%s)", b.expr, other.expr)}
}

func (b Builder) OrExpr(other Builder) Builder {
	return Builder{expr: fmt.Sprintf("(%s) or (%s)", b.expr, other.expr)}
}

func (b Builder) Not() Builder { return Builder{expr: "not (" + b.expr + ")"} }

func (b Builder) String() string { return b.expr }

func (b Builder) Compile(opts ...Option) (*Expression, error) { return Compile(b.expr, opts...) }

func literal(v any) string {
	switch x := v.(type) {
	case string:
		return strconv.Quote(x)
	case []any:
		parts := make([]string, len(x))
		for i, it := range x {
			parts[i] = literal(it)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []string:
		parts := make([]string, len(x))
		for i, it := range x {
			parts[i] = literal(it)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case nil:
		return "null"
	default:
		rv := reflect.ValueOf(v)
		if rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) {
			parts := make([]string, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				parts[i] = literal(rv.Index(i).Interface())
			}
			return "[" + strings.Join(parts, ", ") + "]"
		}
		return fmt.Sprint(x)
	}
}
