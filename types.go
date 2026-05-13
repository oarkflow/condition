package condition

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	interpreterast "github.com/oarkflow/interpreter/pkg/ast"
	interpretereval "github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	interpreterobject "github.com/oarkflow/interpreter/pkg/object"
	interpreterparser "github.com/oarkflow/interpreter/pkg/parser"

	_ "github.com/oarkflow/interpreter/pkg/builtins"
)

type Facts interface {
	Get(path string) (any, bool)
}

type PathFacts interface {
	GetPath(parts []string) (any, bool)
}

type Option func(*config)

type config struct {
	strict        bool
	trace         bool
	vars          Facts
	interpolation Facts
	funcs         *FunctionRegistry
	ops           *OperatorRegistry
}

func defaultConfig() config {
	return config{funcs: GlobalFunctions(), ops: GlobalOperators()}
}

func Strict(v bool) Option { return func(c *config) { c.strict = v } }

func WithTrace(v bool) Option { return func(c *config) { c.trace = v } }

func WithVariables(vars Facts) Option {
	return func(c *config) { c.vars = vars }
}

func WithVariableMap(vars map[string]any) Option {
	return func(c *config) { c.vars = MapFacts(vars) }
}

func WithInterpolation(vars Facts) Option {
	return func(c *config) { c.interpolation = vars }
}

func WithInterpolationMap(vars map[string]any) Option {
	return func(c *config) { c.interpolation = MapFacts(vars) }
}

func WithFunctions(r *FunctionRegistry) Option {
	return func(c *config) {
		if r != nil {
			c.funcs = r
		}
	}
}

// WithOperators is retained for source compatibility. SPL conditions use the
// interpreter's fixed infix operators, so custom operators are not applied.
func WithOperators(r *OperatorRegistry) Option {
	return func(c *config) {
		if r != nil {
			c.ops = r
		}
	}
}

type Expression struct {
	source  string
	program *interpreterast.Program
	expr    interpreterast.Expression
	native  nativeBool
	paths   []string
	roots   []string
	cfg     config
}

type Result struct {
	Matched         bool             `json:"matched"`
	MatchedRuleIDs  []RuleID         `json:"matched_rule_ids,omitempty"`
	Score           float64          `json:"score,omitempty"`
	ScoreValue      int64            `json:"score_value,omitempty"`
	Allowed         bool             `json:"allowed,omitempty"`
	Decision        string           `json:"decision,omitempty"`
	Actions         []Action         `json:"actions,omitempty"`
	CompiledActions []CompiledAction `json:"compiled_actions,omitempty"`
	Events          []Event          `json:"events,omitempty"`
	Trace           Trace            `json:"trace,omitempty"`
}

type Action struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
}

type Event struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
}

type Trace struct {
	Enabled  bool        `json:"enabled,omitempty"`
	Duration string      `json:"duration,omitempty"`
	Steps    []TraceStep `json:"steps,omitempty"`
}

type TraceStep struct {
	Expr    string `json:"expr,omitempty"`
	Op      string `json:"op,omitempty"`
	Path    string `json:"path,omitempty"`
	Value   any    `json:"value,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
}

func Compile(expr string, opts ...Option) (*Expression, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	parseExpr, err := normalizeExpressionVariables(expr, cfg.interpolation)
	if err != nil {
		return nil, err
	}
	l := lexer.NewLexer(parseExpr)
	p := interpreterparser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		return nil, newError(ErrParse, 0, "parser errors: %v", p.Errors())
	}
	root, err := singleExpression(program)
	if err != nil {
		return nil, err
	}
	paths, roots := collectInterpreterPaths(root)
	native := compileNativeBool(root)
	if cfg.funcs != GlobalFunctions() && hasInterpreterCall(root) {
		native = nil
	}
	return &Expression{source: expr, program: program, expr: root, native: native, paths: paths, roots: roots, cfg: cfg}, nil
}

func normalizeExpressionVariables(expr string, interpolation Facts) (string, error) {
	var out strings.Builder
	changed := false
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		switch ch {
		case '"', '\'':
			start := i
			i++
			for i < len(expr) {
				if expr[i] == '\\' {
					i += 2
					continue
				}
				if expr[i] == ch {
					break
				}
				i++
			}
			if changed {
				out.WriteString(expr[start:min(i+1, len(expr))])
			}
		case '`':
			start := i
			i++
			for i < len(expr) {
				if expr[i] == '\\' {
					i += 2
					continue
				}
				if expr[i] == '`' {
					break
				}
				i++
			}
			if changed {
				out.WriteString(expr[start:min(i+1, len(expr))])
			}
		case '$':
			if i+1 < len(expr) && expr[i+1] == '{' {
				end := strings.IndexByte(expr[i+2:], '}')
				if end >= 0 {
					raw := expr[i+2 : i+2+end]
					replacement, err := interpolationReplacement(raw, interpolation)
					if err != nil {
						return "", err
					}
					if !changed {
						changed = true
						out.Grow(len(expr))
						out.WriteString(expr[:i])
					}
					out.WriteString(replacement)
					i += end + 2
					continue
				}
			}
			if i+1 < len(expr) && isIdentifierStart(expr[i+1]) {
				if !changed {
					changed = true
					out.Grow(len(expr))
					out.WriteString(expr[:i])
				}
				continue
			}
			if changed {
				out.WriteByte(ch)
			}
		case '{':
			if i+1 < len(expr) && expr[i+1] == '{' {
				end := strings.Index(expr[i+2:], "}}")
				if end >= 0 {
					raw := expr[i+2 : i+2+end]
					replacement, err := interpolationReplacement(raw, interpolation)
					if err != nil {
						return "", err
					}
					if !changed {
						changed = true
						out.Grow(len(expr))
						out.WriteString(expr[:i])
					}
					out.WriteString(replacement)
					i += end + 3
					continue
				}
			}
			if changed {
				out.WriteByte(ch)
			}
		default:
			if changed {
				out.WriteByte(ch)
			}
		}
	}
	if !changed {
		return expr, nil
	}
	return out.String(), nil
}

func interpolationReplacement(raw string, interpolation Facts) (string, error) {
	name := normalizePlaceholderName(raw)
	if name == "" {
		return "", newError(ErrParse, 0, "empty interpolation placeholder")
	}
	if interpolation == nil {
		return name, nil
	}
	v, ok := interpolation.Get(name)
	if !ok {
		return "", newError(ErrMissing, 0, "missing interpolation variable %q", name)
	}
	return literal(v), nil
}

func normalizePlaceholderName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "$")
	name = strings.TrimPrefix(name, ":")
	return strings.TrimSpace(name)
}

func isIdentifierStart(ch byte) bool {
	return ch == '_' || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z')
}

func MustCompile(expr string, opts ...Option) *Expression {
	e, err := Compile(expr, opts...)
	if err != nil {
		panic(err)
	}
	return e
}

func Eval(expr string, facts Facts) (Result, error) {
	e, err := Compile(expr)
	if err != nil {
		return Result{}, err
	}
	return e.Eval(context.Background(), facts)
}

func (e *Expression) String() string { return e.source }

func (e *Expression) Eval(ctx context.Context, facts Facts) (Result, error) {
	return e.EvalWithVariables(ctx, facts, nil)
}

func (e *Expression) EvalBool(ctx context.Context, facts Facts) (bool, error) {
	res, err := e.eval(ctx, facts, nil, false)
	return res.Matched, err
}

func (e *Expression) EvalWithVariables(ctx context.Context, facts Facts, vars Facts) (Result, error) {
	return e.eval(ctx, facts, vars, true)
}

func (e *Expression) eval(ctx context.Context, facts Facts, vars Facts, full bool) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if facts == nil {
		facts = MapFacts(nil)
	}
	if vars == nil {
		vars = e.cfg.vars
	}
	var tr *Trace
	if full && e.cfg.trace {
		tr = &Trace{Enabled: true}
	}
	if tr != nil {
		tr.Enabled = true
	}
	start := time.Now()
	if e.native != nil {
		nativeFacts := facts
		if vars != nil {
			nativeFacts = ChainedFacts{vars, facts}
		}
		rf := rankingFacts{global: nativeFacts, provider: nativeFacts}
		matched, err := e.native(&rf, vars, e.cfg.strict)
		res := Result{Matched: matched}
		if tr != nil {
			tr.Duration = time.Since(start).String()
			tr.Steps = append(tr.Steps, TraceStep{Expr: e.source, Result: matched})
			res.Trace = *tr
		}
		if err != nil {
			return res, err
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		return res, nil
	}
	env := e.newInterpreterEnv(ctx, facts, vars)
	v := interpretereval.Eval(e.program, env)
	native := interpreterObjectToNative(v)
	res := Result{Matched: interpreterTruthy(v)}
	if tr != nil {
		tr.Duration = time.Since(start).String()
		tr.Steps = append(tr.Steps, TraceStep{Expr: e.source, Result: native})
		res.Trace = *tr
	}
	if err := e.interpreterError(v); err != nil {
		return res, err
	}
	return res, nil
}

func singleExpression(program *interpreterast.Program) (interpreterast.Expression, error) {
	if program == nil || len(program.Statements) != 1 {
		return nil, newError(ErrParse, 0, "condition must be a single expression")
	}
	stmt, ok := program.Statements[0].(*interpreterast.ExpressionStatement)
	if !ok || stmt.Expression == nil {
		return nil, newError(ErrParse, 0, "condition must be a single expression")
	}
	return stmt.Expression, nil
}

func hasInterpreterCall(n interpreterast.Expression) bool {
	switch x := n.(type) {
	case *interpreterast.CallExpression:
		return true
	case *interpreterast.InfixExpression:
		return hasInterpreterCall(x.Left) || hasInterpreterCall(x.Right)
	case *interpreterast.PrefixExpression:
		return hasInterpreterCall(x.Right)
	case *interpreterast.IndexExpression:
		return hasInterpreterCall(x.Left) || hasInterpreterCall(x.Index)
	default:
		return false
	}
}

func (e *Expression) newInterpreterEnv(ctx context.Context, facts Facts, vars Facts) *interpreterobject.Environment {
	env := interpreterobject.NewEnvironment()
	env.RuntimeLimits = &interpreterobject.RuntimeLimits{Ctx: ctx, HeapCheckEvery: 128}
	injectFacts(env, facts, e.paths, e.roots, e.cfg.strict)
	if vars != nil {
		injectFacts(env, vars, e.paths, e.roots, true)
	}
	e.injectFunctions(env, ctx, facts)
	return env
}

func (e *Expression) injectFunctions(env *interpreterobject.Environment, ctx context.Context, facts Facts) {
	if e.cfg.funcs == nil {
		return
	}
	e.cfg.funcs.mu.RLock()
	defer e.cfg.funcs.mu.RUnlock()
	for name, fn := range e.cfg.funcs.m {
		if fn == nil {
			continue
		}
		captured := fn
		builtin := &interpreterobject.Builtin{Fn: func(args ...interpreterobject.Object) interpreterobject.Object {
			nativeArgs := make([]any, len(args))
			for i, arg := range args {
				nativeArgs[i] = interpreterObjectToNative(arg)
			}
			out, err := captured(EvalContext{Context: ctx, Facts: facts}, nativeArgs...)
			if err != nil {
				return interpreterobject.NewError("%s", err.Error())
			}
			return nativeToInterpreterObject(out)
		}}
		env.Set(name, builtin)
		for _, alias := range functionAliases(name) {
			env.Set(alias, builtin)
		}
	}
}

func functionAliases(name string) []string {
	switch strings.ToLower(name) {
	case "countwhere":
		return []string{"countWhere"}
	case "groupby":
		return []string{"groupBy"}
	case "groupcount":
		return []string{"groupCount"}
	case "groupsum":
		return []string{"groupSum"}
	case "groupavg":
		return []string{"groupAvg"}
	case "groupmin":
		return []string{"groupMin"}
	case "groupmax":
		return []string{"groupMax"}
	case "groupcountwhere":
		return []string{"groupCountWhere"}
	case "groupsumwhere":
		return []string{"groupSumWhere"}
	case "groupavgwhere":
		return []string{"groupAvgWhere"}
	case "distinctcount":
		return []string{"distinctCount"}
	case "isnull":
		return []string{"isNull"}
	case "isnotnull":
		return []string{"isNotNull"}
	case "hasprefix":
		return []string{"hasPrefix"}
	case "hassuffix":
		return []string{"hasSuffix"}
	case "stablebucket":
		return []string{"stableBucket"}
	case "nullablematch":
		return []string{"nullableMatch"}
	case "rangematch":
		return []string{"rangeMatch"}
	case "validnow":
		return []string{"validNow"}
	case "betweentime":
		return []string{"betweenTime"}
	case "default":
		return []string{"defaultValue"}
	default:
		return nil
	}
}

func (e *Expression) interpreterError(v interpreterobject.Object) error {
	errObj, ok := v.(*interpreterobject.Error)
	if !ok {
		return nil
	}
	msg := errObj.Message
	if !e.cfg.strict && nonStrictInterpreterError(msg) {
		return nil
	}
	return &Error{Kind: classifyInterpreterError(msg), Message: msg}
}

func nonStrictInterpreterError(msg string) bool {
	return strings.Contains(msg, "identifier not found") ||
		strings.Contains(msg, "property or method") ||
		strings.Contains(msg, "type mismatch") ||
		strings.Contains(msg, "membership not supported") ||
		strings.Contains(msg, "unknown operator")
}

func classifyInterpreterError(msg string) ErrorKind {
	switch {
	case strings.Contains(msg, "identifier not found"), strings.Contains(msg, "property or method"):
		return ErrMissing
	case strings.Contains(msg, "type mismatch"), strings.Contains(msg, "membership not supported"):
		return ErrType
	default:
		return ErrEval
	}
}

func injectFacts(env *interpreterobject.Environment, facts Facts, paths []string, roots []string, strict bool) {
	if facts == nil {
		facts = MapFacts(nil)
	}
	for _, root := range roots {
		if v, ok := facts.Get(root); ok {
			env.Set(root, nativeToInterpreterObject(v))
		} else if !strict {
			env.Set(root, &interpreterobject.Hash{Pairs: map[interpreterobject.HashKey]interpreterobject.HashPair{}})
		}
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if v, ok := facts.Get(path); ok {
			setInterpreterPath(env, strings.Split(path, "."), v)
		}
	}
}

func setInterpreterPath(env *interpreterobject.Environment, parts []string, value any) {
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		env.Set(parts[0], nativeToInterpreterObject(value))
		return
	}
	rootObj, ok := env.Get(parts[0])
	root, ok := rootObj.(*interpreterobject.Hash)
	if !ok {
		root = &interpreterobject.Hash{Pairs: map[interpreterobject.HashKey]interpreterobject.HashPair{}}
		env.Set(parts[0], root)
	}
	setHashPath(root, parts[1:], nativeToInterpreterObject(value))
}

func setHashPath(h *interpreterobject.Hash, parts []string, value interpreterobject.Object) {
	if len(parts) == 0 {
		return
	}
	key := &interpreterobject.String{Value: parts[0]}
	if len(parts) == 1 {
		h.Pairs[key.HashKey()] = interpreterobject.HashPair{Key: key, Value: value}
		return
	}
	childObj, ok := h.Pairs[key.HashKey()]
	child, ok := childObj.Value.(*interpreterobject.Hash)
	if !ok {
		child = &interpreterobject.Hash{Pairs: map[interpreterobject.HashKey]interpreterobject.HashPair{}}
		h.Pairs[key.HashKey()] = interpreterobject.HashPair{Key: key, Value: child}
	}
	setHashPath(child, parts[1:], value)
}

func nativeToInterpreterObject(v any) interpreterobject.Object {
	if v == nil {
		return interpreterobject.NULL
	}
	if obj, ok := v.(interpreterobject.Object); ok {
		return obj
	}
	switch x := v.(type) {
	case bool:
		return interpreterobject.NativeBoolToBooleanObject(x)
	case int:
		return &interpreterobject.Integer{Value: int64(x)}
	case int8:
		return &interpreterobject.Integer{Value: int64(x)}
	case int16:
		return &interpreterobject.Integer{Value: int64(x)}
	case int32:
		return &interpreterobject.Integer{Value: int64(x)}
	case int64:
		return &interpreterobject.Integer{Value: x}
	case uint:
		return &interpreterobject.Integer{Value: int64(x)}
	case uint8:
		return &interpreterobject.Integer{Value: int64(x)}
	case uint16:
		return &interpreterobject.Integer{Value: int64(x)}
	case uint32:
		return &interpreterobject.Integer{Value: int64(x)}
	case uint64:
		return &interpreterobject.Integer{Value: int64(x)}
	case float32:
		return &interpreterobject.Float{Value: float64(x)}
	case float64:
		return &interpreterobject.Float{Value: x}
	case string:
		return &interpreterobject.String{Value: x}
	case []any:
		items := make([]interpreterobject.Object, len(x))
		for i := range x {
			items[i] = nativeToInterpreterObject(x[i])
		}
		return &interpreterobject.Array{Elements: items}
	case []string:
		items := make([]interpreterobject.Object, len(x))
		for i := range x {
			items[i] = &interpreterobject.String{Value: x[i]}
		}
		return &interpreterobject.Array{Elements: items}
	case map[string]any:
		return nativeMapToInterpreterObject(x)
	case MapFacts:
		return nativeMapToInterpreterObject(map[string]any(x))
	case time.Time:
		return &interpreterobject.String{Value: x.Format(time.RFC3339Nano)}
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return interpreterobject.NULL
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Bool:
		return interpreterobject.NativeBoolToBooleanObject(rv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &interpreterobject.Integer{Value: rv.Int()}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &interpreterobject.Integer{Value: int64(rv.Uint())}
	case reflect.Float32, reflect.Float64:
		return &interpreterobject.Float{Value: rv.Float()}
	case reflect.String:
		return &interpreterobject.String{Value: rv.String()}
	case reflect.Slice, reflect.Array:
		items := make([]interpreterobject.Object, rv.Len())
		for i := range items {
			items[i] = nativeToInterpreterObject(rv.Index(i).Interface())
		}
		return &interpreterobject.Array{Elements: items}
	case reflect.Map:
		pairs := make(map[interpreterobject.HashKey]interpreterobject.HashPair, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			key := nativeToInterpreterObject(iter.Key().Interface())
			hashKey, ok := key.(interpreterobject.Hashable)
			if !ok {
				continue
			}
			pairs[hashKey.HashKey()] = interpreterobject.HashPair{Key: key, Value: nativeToInterpreterObject(iter.Value().Interface())}
		}
		return &interpreterobject.Hash{Pairs: pairs}
	case reflect.Struct:
		pairs := make(map[interpreterobject.HashKey]interpreterobject.HashPair, rv.NumField())
		t := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := field.Name
			if tag := field.Tag.Get("json"); tag != "" {
				if p := strings.Split(tag, ",")[0]; p != "" && p != "-" {
					name = p
				}
			}
			key := &interpreterobject.String{Value: name}
			pairs[key.HashKey()] = interpreterobject.HashPair{Key: key, Value: nativeToInterpreterObject(rv.Field(i).Interface())}
		}
		return &interpreterobject.Hash{Pairs: pairs}
	default:
		return &interpreterobject.String{Value: fmt.Sprint(v)}
	}
}

func nativeMapToInterpreterObject(m map[string]any) interpreterobject.Object {
	pairs := make(map[interpreterobject.HashKey]interpreterobject.HashPair, len(m))
	for k, v := range m {
		key := &interpreterobject.String{Value: k}
		pairs[key.HashKey()] = interpreterobject.HashPair{Key: key, Value: nativeToInterpreterObject(v)}
	}
	return &interpreterobject.Hash{Pairs: pairs}
}

func interpreterObjectToNative(obj interpreterobject.Object) any {
	switch x := obj.(type) {
	case nil:
		return nil
	case *interpreterobject.Null:
		return nil
	case *interpreterobject.Boolean:
		return x.Value
	case *interpreterobject.Integer:
		return x.Value
	case *interpreterobject.Float:
		return x.Value
	case *interpreterobject.String:
		return x.Value
	case *interpreterobject.Array:
		out := make([]any, len(x.Elements))
		for i := range x.Elements {
			out[i] = interpreterObjectToNative(x.Elements[i])
		}
		return out
	case *interpreterobject.Hash:
		out := make(map[string]any, len(x.Pairs))
		for _, pair := range x.Pairs {
			if key, ok := pair.Key.(*interpreterobject.String); ok {
				out[key.Value] = interpreterObjectToNative(pair.Value)
			}
		}
		return out
	case *interpreterobject.Error:
		return x.Message
	default:
		return obj.Inspect()
	}
}

func interpreterTruthy(obj interpreterobject.Object) bool {
	if obj == nil {
		return false
	}
	if errObj, ok := obj.(*interpreterobject.Error); ok {
		return !nonStrictInterpreterError(errObj.Message)
	}
	return interpreterobject.IsTruthy(obj)
}

func collectInterpreterPaths(expr interpreterast.Expression) ([]string, []string) {
	paths := map[string]struct{}{}
	roots := map[string]struct{}{}
	var walk func(interpreterast.Expression, bool)
	walk = func(n interpreterast.Expression, skipIdentifier bool) {
		switch x := n.(type) {
		case *interpreterast.Identifier:
			if !skipIdentifier {
				roots[x.Name] = struct{}{}
				paths[x.Name] = struct{}{}
			}
		case *interpreterast.DotExpression:
			if p, ok := interpreterPath(x); ok {
				roots[strings.Split(p, ".")[0]] = struct{}{}
				paths[p] = struct{}{}
				return
			}
			walk(x.Left, false)
		case *interpreterast.IndexExpression:
			if p, ok := interpreterPath(x); ok {
				roots[strings.Split(p, ".")[0]] = struct{}{}
				paths[p] = struct{}{}
				return
			}
			walk(x.Left, false)
			walk(x.Index, false)
		case *interpreterast.PrefixExpression:
			walk(x.Right, false)
		case *interpreterast.InfixExpression:
			walk(x.Left, false)
			walk(x.Right, false)
		case *interpreterast.ArrayLiteral:
			for _, item := range x.Elements {
				walk(item, false)
			}
		case *interpreterast.HashLiteral:
			for _, item := range x.Entries {
				walk(item.Key, false)
				walk(item.Value, false)
			}
		case *interpreterast.CallExpression:
			walk(x.Function, true)
			for _, arg := range x.Arguments {
				walk(arg, false)
			}
		case *interpreterast.TernaryExpression:
			walk(x.Condition, false)
			walk(x.Consequence, false)
			walk(x.Alternative, false)
		}
	}
	walk(expr, false)
	return sortedKeys(paths), sortedKeys(roots)
}

func interpreterPath(expr interpreterast.Expression) (string, bool) {
	parts, ok := interpreterPathParts(expr)
	if !ok || len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "."), true
}

func interpreterPathParts(expr interpreterast.Expression) ([]string, bool) {
	switch x := expr.(type) {
	case *interpreterast.Identifier:
		return []string{x.Name}, true
	case *interpreterast.DotExpression:
		parts, ok := interpreterPathParts(x.Left)
		if !ok || x.Right == nil {
			return nil, false
		}
		return append(parts, x.Right.Name), true
	case *interpreterast.IndexExpression:
		parts, ok := interpreterPathParts(x.Left)
		if !ok {
			return nil, false
		}
		switch idx := x.Index.(type) {
		case *interpreterast.StringLiteral:
			return append(parts, idx.Value), true
		case *interpreterast.IntegerLiteral:
			return append(parts, fmt.Sprintf("%d", idx.Value)), true
		case *interpreterast.Identifier:
			return append(parts, idx.Name), true
		default:
			return nil, false
		}
	default:
		return nil, false
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
