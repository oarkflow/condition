package condition

import (
	"hash/fnv"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	interpreterast "github.com/oarkflow/interpreter/pkg/ast"
)

type FieldID uint16
type StringID uint32
type NamespaceID uint32
type RuleID uint64

type OpCode uint8

const (
	OpEq OpCode = iota
	OpNeq
	OpGt
	OpGte
	OpLt
	OpLte
	OpIn
	OpNotIn
	OpBetween
	OpExists
	OpNotExists
	OpAnd
	OpOr
	OpNot
	OpJumpIfFalse
	OpJumpIfTrue
)

type ExecutionMode uint8

const (
	FirstMatch ExecutionMode = iota
	AllMatches
	HighestPriority
	DenyOverrides
	AllowOverrides
	ScoreBased
)

type ValueKind uint8

const (
	ValueMissing ValueKind = iota
	ValueInt
	ValueFloat
	ValueString
	ValueBool
	ValueTime
)

type Instruction struct {
	Op    OpCode
	Field FieldID
	Kind  ValueKind
	A     int64
	B     int64
	SetID uint32
	Jump  int16
}

type ActionType uint16

const (
	ActionAllow ActionType = iota + 1
	ActionDeny
	ActionScore
	ActionRequireApproval
	ActionNotify
	ActionAssign
	ActionRoute
	ActionSetField
	ActionEscalate
	ActionGenerateDocument
	ActionWebhook
)

type CompiledAction struct {
	Type  ActionType `json:"type"`
	Key   uint32     `json:"key,omitempty"`
	Value int64      `json:"value,omitempty"`
	Ref   uint32     `json:"ref,omitempty"`
}

type Registry struct {
	fields     map[string]FieldID
	fieldNames []string
	namespaces map[string]NamespaceID
	nsNames    []string
	strings    map[string]StringID
	stringVals []string
	actions    map[string]ActionType
	actionRefs map[string]uint32
	actionVals []string
}

func NewRegistry() *Registry {
	r := &Registry{
		fields:     make(map[string]FieldID),
		namespaces: make(map[string]NamespaceID),
		strings:    make(map[string]StringID),
		actions:    make(map[string]ActionType),
		actionRefs: make(map[string]uint32),
	}
	r.actions["allow"] = ActionAllow
	r.actions["deny"] = ActionDeny
	r.actions["score"] = ActionScore
	r.actions["require_approval"] = ActionRequireApproval
	r.actions["notify"] = ActionNotify
	r.actions["assign"] = ActionAssign
	r.actions["route"] = ActionRoute
	r.actions["set_field"] = ActionSetField
	r.actions["escalate"] = ActionEscalate
	r.actions["generate_document"] = ActionGenerateDocument
	r.actions["webhook"] = ActionWebhook
	return r
}

func (r *Registry) Field(name string) FieldID {
	if id, ok := r.fields[name]; ok {
		return id
	}
	id := FieldID(len(r.fieldNames) + 1)
	r.fields[name] = id
	r.fieldNames = append(r.fieldNames, name)
	return id
}

func (r *Registry) FieldName(id FieldID) string {
	i := int(id) - 1
	if i < 0 || i >= len(r.fieldNames) {
		return ""
	}
	return r.fieldNames[i]
}

func (r *Registry) MaxField() FieldID {
	return FieldID(len(r.fieldNames))
}

func (r *Registry) Namespace(name string) NamespaceID {
	if id, ok := r.namespaces[name]; ok {
		return id
	}
	id := NamespaceID(len(r.nsNames) + 1)
	r.namespaces[name] = id
	r.nsNames = append(r.nsNames, name)
	return id
}

func (r *Registry) Intern(s string) StringID {
	if id, ok := r.strings[s]; ok {
		return id
	}
	id := StringID(len(r.stringVals) + 1)
	r.strings[s] = id
	r.stringVals = append(r.stringVals, s)
	return id
}

func (r *Registry) String(id StringID) string {
	i := int(id) - 1
	if i < 0 || i >= len(r.stringVals) {
		return ""
	}
	return r.stringVals[i]
}

func (r *Registry) actionType(name string) ActionType {
	if t, ok := r.actions[name]; ok {
		return t
	}
	return ActionType(r.actionRef(name))
}

func (r *Registry) actionRef(s string) uint32 {
	if id, ok := r.actionRefs[s]; ok {
		return id
	}
	id := uint32(len(r.actionVals) + 1)
	r.actionRefs[s] = id
	r.actionVals = append(r.actionVals, s)
	return id
}

type TypedFacts struct {
	kinds   []ValueKind
	ints    []int64
	floats  []float64
	strings []StringID
	bools   []bool
	times   []int64
}

func NewTypedFacts(max FieldID) *TypedFacts {
	f := &TypedFacts{}
	f.Grow(max)
	return f
}

func (f *TypedFacts) Grow(max FieldID) {
	n := int(max) + 1
	if len(f.kinds) >= n {
		return
	}
	f.kinds = growKinds(f.kinds, n)
	f.ints = growInt64s(f.ints, n)
	f.floats = growFloats(f.floats, n)
	f.strings = growStrings(f.strings, n)
	f.bools = growBools(f.bools, n)
	f.times = growInt64s(f.times, n)
}

func (f *TypedFacts) Reset() {
	for i := range f.kinds {
		f.kinds[i] = ValueMissing
	}
}

func (f *TypedFacts) SetInt(id FieldID, v int64) { f.Grow(id); f.kinds[id] = ValueInt; f.ints[id] = v }
func (f *TypedFacts) SetFloat(id FieldID, v float64) {
	f.Grow(id)
	f.kinds[id] = ValueFloat
	f.floats[id] = v
}
func (f *TypedFacts) SetString(id FieldID, v StringID) {
	f.Grow(id)
	f.kinds[id] = ValueString
	f.strings[id] = v
}
func (f *TypedFacts) SetBool(id FieldID, v bool) {
	f.Grow(id)
	f.kinds[id] = ValueBool
	f.bools[id] = v
}
func (f *TypedFacts) SetTime(id FieldID, unix int64) {
	f.Grow(id)
	f.kinds[id] = ValueTime
	f.times[id] = unix
}

func (f *TypedFacts) kind(id FieldID) ValueKind {
	if int(id) >= len(f.kinds) {
		return ValueMissing
	}
	return f.kinds[id]
}

type compiledSet struct {
	Kind    ValueKind
	Ints    []int64
	Floats  []float64
	Strings []StringID
	Bools   []bool
}

type CompiledRule struct {
	ID           RuleID
	Namespace    NamespaceID
	Priority     int32
	Salience     int32
	Specificity  int32
	ValidFrom    int64
	ValidUntil   int64
	Instructions []Instruction
	Actions      []CompiledAction
	Decision     StringID
	Mode         ExecutionMode
	indexed      bool
	indexKey     indexKey
}

type Program struct {
	registry           *Registry
	rules              []CompiledRule
	rulesByNamespace   map[NamespaceID][]int
	defaultByNamespace map[NamespaceID][]int
	eqIndex            map[NamespaceID]map[indexKey][]int
	sets               []compiledSet
	defaultMode        ExecutionMode
}

type indexKey struct {
	Field FieldID
	Kind  ValueKind
	A     int64
}

type Runtime struct {
	engine atomic.Pointer[Program]
}

type CompileOption func(*compileConfig)

type compileConfig struct {
	mode        ExecutionMode
	now         int64
	exprOptions []Option
}

func WithExecutionMode(mode ExecutionMode) CompileOption {
	return func(c *compileConfig) { c.mode = mode }
}

func WithCompileExpressionOptions(opts ...Option) CompileOption {
	return func(c *compileConfig) { c.exprOptions = append(c.exprOptions, opts...) }
}

func CompileRuleSet(input RuleSet, registry *Registry, opts ...CompileOption) (*Program, error) {
	if registry == nil {
		registry = NewRegistry()
	}
	cfg := compileConfig{mode: AllMatches}
	for _, opt := range opts {
		opt(&cfg)
	}
	if input.ExecutionMode != 0 {
		cfg.mode = input.ExecutionMode
	}
	p := &Program{
		registry:           registry,
		defaultMode:        cfg.mode,
		rulesByNamespace:   make(map[NamespaceID][]int),
		defaultByNamespace: make(map[NamespaceID][]int),
		eqIndex:            make(map[NamespaceID]map[indexKey][]int),
	}
	namespaceName := input.Namespace
	if namespaceName == "" {
		namespaceName = input.Name
	}
	ns := registry.Namespace(namespaceName)
	for _, rule := range input.Rules {
		cr, err := compileBytecodeRule(rule, ns, registry, p, cfg.mode, cfg.exprOptions)
		if err != nil {
			return nil, err
		}
		if rule.Enabled != nil && !*rule.Enabled {
			continue
		}
		idx := len(p.rules)
		p.rules = append(p.rules, cr)
		p.rulesByNamespace[cr.Namespace] = append(p.rulesByNamespace[cr.Namespace], idx)
		if cr.indexed {
			m := p.eqIndex[cr.Namespace]
			if m == nil {
				m = make(map[indexKey][]int)
				p.eqIndex[cr.Namespace] = m
			}
			m[cr.indexKey] = append(m[cr.indexKey], idx)
		} else {
			p.defaultByNamespace[cr.Namespace] = append(p.defaultByNamespace[cr.Namespace], idx)
		}
	}
	for ns, indexes := range p.rulesByNamespace {
		sortRuleIndexes(indexes, p.rules)
		p.rulesByNamespace[ns] = indexes
	}
	for ns, indexes := range p.defaultByNamespace {
		sortRuleIndexes(indexes, p.rules)
		p.defaultByNamespace[ns] = indexes
	}
	for _, buckets := range p.eqIndex {
		for key, indexes := range buckets {
			sortRuleIndexes(indexes, p.rules)
			buckets[key] = indexes
		}
	}
	return p, nil
}

func NewRuntime(program *Program) *Runtime {
	r := &Runtime{}
	r.engine.Store(program)
	return r
}

func (r *Runtime) Reload(program *Program) {
	r.engine.Store(program)
}

func (r *Runtime) Evaluate(ns NamespaceID, facts *TypedFacts, ctx *EvalContext) Result {
	p := r.engine.Load()
	if p == nil {
		return Result{}
	}
	return p.Evaluate(ns, facts, ctx)
}

func (r *Runtime) EvaluateAny(ns NamespaceID, facts any, ctx *EvalContext) (Result, error) {
	p := r.engine.Load()
	if p == nil {
		return Result{}, nil
	}
	return p.EvaluateAny(ns, facts, ctx)
}

func (p *Program) Evaluate(ns NamespaceID, facts *TypedFacts, ctx *EvalContext) Result {
	if ctx == nil {
		ctx = &EvalContext{}
	}
	ctx.Matched = ctx.Matched[:0]
	ctx.Actions = ctx.Actions[:0]
	ctx.Stack = ctx.Stack[:0]
	var out Result
	now := time.Now().Unix()
	candidates := p.candidates(ns, facts, ctx)
	for _, idx := range candidates {
		rule := &p.rules[idx]
		if rule.ValidFrom > 0 && now < rule.ValidFrom {
			continue
		}
		if rule.ValidUntil > 0 && now > rule.ValidUntil {
			continue
		}
		if !evalInstructions(rule.Instructions, facts, ctx, p.sets) {
			continue
		}
		out.Matched = true
		out.MatchedRuleIDs = append(ctx.Matched, rule.ID)
		ctx.Matched = out.MatchedRuleIDs
		ctx.Actions = append(ctx.Actions, rule.Actions...)
		out.CompiledActions = ctx.Actions
		for _, a := range rule.Actions {
			switch a.Type {
			case ActionAllow:
				out.Allowed = true
			case ActionDeny:
				out.Allowed = false
			case ActionScore:
				out.ScoreValue += a.Value
				out.Score += float64(a.Value)
			}
		}
		if rule.Decision != 0 {
			out.Decision = p.registry.String(rule.Decision)
		}
		if rule.Mode == FirstMatch || p.defaultMode == FirstMatch || rule.Mode == HighestPriority || p.defaultMode == HighestPriority {
			return out
		}
		if rule.Mode == DenyOverrides || p.defaultMode == DenyOverrides {
			if compiledRuleDenies(rule) {
				return out
			}
		}
		if rule.Mode == AllowOverrides || p.defaultMode == AllowOverrides {
			if compiledRuleAllows(rule) {
				return out
			}
		}
	}
	return out
}

func (p *Program) EvaluateAny(ns NamespaceID, facts any, ctx *EvalContext) (Result, error) {
	if typed, ok := facts.(*TypedFacts); ok {
		return p.Evaluate(ns, typed, ctx), nil
	}
	if typed, ok := facts.(TypedFacts); ok {
		return p.Evaluate(ns, &typed, ctx), nil
	}
	if ctx == nil {
		ctx = &EvalContext{}
	}
	if ctx.TypedFacts == nil {
		ctx.TypedFacts = NewTypedFacts(p.registry.MaxField())
	}
	ctx.TypedFacts.Reset()
	if err := AnyFactsAdapter(p.registry, facts, ctx.TypedFacts); err != nil {
		return Result{}, err
	}
	return p.Evaluate(ns, ctx.TypedFacts, ctx), nil
}

func (p *Program) candidates(ns NamespaceID, facts *TypedFacts, ctx *EvalContext) []int {
	index := p.eqIndex[ns]
	if len(index) == 0 || facts == nil {
		return p.rulesByNamespace[ns]
	}
	out := ctx.Candidates[:0]
	out = append(out, p.defaultByNamespace[ns]...)
	for key, rules := range index {
		if typedFactEqualsKey(facts, key) {
			out = append(out, rules...)
		}
	}
	if len(out) > 1 {
		sortRuleIndexes(out, p.rules)
	}
	ctx.Candidates = out
	return out
}

func typedFactEqualsKey(facts *TypedFacts, key indexKey) bool {
	if facts.kind(key.Field) == ValueMissing {
		return false
	}
	ins := Instruction{Op: OpEq, Field: key.Field, Kind: key.Kind, A: key.A}
	return evalCompare(ins, facts)
}

func sortRuleIndexes(indexes []int, rules []CompiledRule) {
	sort.SliceStable(indexes, func(i, j int) bool {
		a := rules[indexes[i]]
		b := rules[indexes[j]]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.Salience != b.Salience {
			return a.Salience > b.Salience
		}
		if a.Specificity != b.Specificity {
			return a.Specificity > b.Specificity
		}
		return a.ID < b.ID
	})
}

func compiledRuleAllows(rule *CompiledRule) bool {
	for _, action := range rule.Actions {
		if action.Type == ActionAllow {
			return true
		}
	}
	return false
}

func compiledRuleDenies(rule *CompiledRule) bool {
	for _, action := range rule.Actions {
		if action.Type == ActionDeny {
			return true
		}
	}
	return false
}

type ExpressionProgram struct {
	registry     *Registry
	instructions []Instruction
	sets         []compiledSet
}

func CompileDSL(expr string, registry *Registry, opts ...Option) (*ExpressionProgram, error) {
	if registry == nil {
		registry = NewRegistry()
	}
	compiled, err := Compile(expr, opts...)
	if err != nil {
		return nil, err
	}
	p := &Program{registry: registry}
	ins, err := compileBytecodeExpr(compiled.expr, registry, p)
	if err != nil {
		return nil, err
	}
	return &ExpressionProgram{registry: registry, instructions: ins, sets: p.sets}, nil
}

func (p *ExpressionProgram) EvalBool(facts *TypedFacts, ctx *EvalContext) bool {
	if ctx == nil {
		ctx = &EvalContext{}
	}
	ctx.Stack = ctx.Stack[:0]
	return evalInstructions(p.instructions, facts, ctx, p.sets)
}

func (p *ExpressionProgram) EvalAny(facts any, ctx *EvalContext) (bool, error) {
	if typed, ok := facts.(*TypedFacts); ok {
		return p.EvalBool(typed, ctx), nil
	}
	if typed, ok := facts.(TypedFacts); ok {
		return p.EvalBool(&typed, ctx), nil
	}
	if ctx == nil {
		ctx = &EvalContext{}
	}
	if ctx.TypedFacts == nil {
		ctx.TypedFacts = NewTypedFacts(p.registry.MaxField())
	}
	ctx.TypedFacts.Reset()
	if err := AnyFactsAdapter(p.registry, facts, ctx.TypedFacts); err != nil {
		return false, err
	}
	return p.EvalBool(ctx.TypedFacts, ctx), nil
}

func MapFactsAdapter(registry *Registry, m map[string]any, out *TypedFacts) error {
	return AnyFactsAdapter(registry, m, out)
}

func StructFactsAdapter(registry *Registry, src any, out *TypedFacts) error {
	return AnyFactsAdapter(registry, src, out)
}

func AnyFactsAdapter(registry *Registry, src any, out *TypedFacts) error {
	if registry == nil || out == nil {
		return newError(ErrEval, 0, "registry and typed facts are required")
	}
	if src == nil {
		return nil
	}
	if typed, ok := src.(*TypedFacts); ok {
		copyTypedFacts(out, typed)
		return nil
	}
	if typed, ok := src.(TypedFacts); ok {
		copyTypedFacts(out, &typed)
		return nil
	}
	return adaptAnyValue(registry, reflect.ValueOf(src), "", out)
}

func compileBytecodeRule(rule Rule, defaultNS NamespaceID, registry *Registry, p *Program, mode ExecutionMode, opts []Option) (CompiledRule, error) {
	ns := defaultNS
	if rule.Namespace != "" {
		ns = registry.Namespace(rule.Namespace)
	}
	id := parseRuleID(rule.ID)
	cr := CompiledRule{
		ID:         id,
		Namespace:  ns,
		Priority:   int32(rule.Priority),
		Salience:   int32(rule.Salience),
		ValidFrom:  rule.ValidFrom,
		ValidUntil: rule.ValidUntil,
		Mode:       mode,
	}
	if rule.Condition != "" {
		expr, err := Compile(rule.Condition, opts...)
		if err != nil {
			return cr, err
		}
		ins, err := compileBytecodeExpr(expr.expr, registry, p)
		if err != nil {
			return cr, err
		}
		cr.Instructions = ins
		cr.Specificity = int32(countSpecificity(expr.expr))
		if key, ok := firstEqualityIndex(expr.expr, registry); ok {
			cr.indexed = true
			cr.indexKey = key
		}
	} else {
		cr.Instructions = []Instruction{{Op: OpExists, Field: 0}}
	}
	cr.Actions = compileActions(registry, rule)
	if rule.Decision != "" {
		cr.Decision = registry.Intern(rule.Decision)
	}
	return cr, nil
}

func compileBytecodeExpr(n interpreterast.Expression, registry *Registry, p *Program) ([]Instruction, error) {
	switch x := n.(type) {
	case *interpreterast.InfixExpression:
		if x.Operator == "&&" || x.Operator == "||" {
			left, err := compileBytecodeExpr(x.Left, registry, p)
			if err != nil {
				return nil, err
			}
			right, err := compileBytecodeExpr(x.Right, registry, p)
			if err != nil {
				return nil, err
			}
			op := OpAnd
			if x.Operator == "||" {
				op = OpOr
			}
			return append(append(left, right...), Instruction{Op: op}), nil
		}
		return compileCompareExpr(x, registry, p)
	case *interpreterast.PrefixExpression:
		if x.Operator != "!" {
			return nil, newError(ErrOperator, 0, "unsupported unary operator %q", x.Operator)
		}
		inner, err := compileBytecodeExpr(x.Right, registry, p)
		if err != nil {
			return nil, err
		}
		return append(inner, Instruction{Op: OpNot}), nil
	case *interpreterast.CallExpression:
		name, ok := callName(x)
		if !ok {
			return nil, newError(ErrFunction, 0, "dynamic function calls are not supported by bytecode compiler")
		}
		switch strings.ToLower(name) {
		case "exists":
			if len(x.Arguments) != 1 {
				return nil, newError(ErrFunction, 0, "exists expects 1 argument")
			}
			path, ok := interpreterPath(x.Arguments[0])
			if !ok {
				return nil, newError(ErrFunction, 0, "exists expects a path")
			}
			return []Instruction{{Op: OpExists, Field: registry.Field(path)}}, nil
		case "between":
			if len(x.Arguments) != 3 {
				return nil, newError(ErrFunction, 0, "between expects field, min, max")
			}
			path, ok := interpreterPath(x.Arguments[0])
			if !ok {
				return nil, newError(ErrFunction, 0, "between expects a path first argument")
			}
			min, ok1 := literalNumber(x.Arguments[1])
			max, ok2 := literalNumber(x.Arguments[2])
			if !ok1 || !ok2 {
				return nil, newError(ErrFunction, 0, "between min and max must be numeric literals")
			}
			return []Instruction{{Op: OpBetween, Field: registry.Field(path), Kind: ValueFloat, A: int64(math.Float64bits(min)), B: int64(math.Float64bits(max))}}, nil
		default:
			return nil, newError(ErrFunction, 0, "function %q is not supported by bytecode compiler", name)
		}
	case *interpreterast.BooleanLiteral:
		if x.Value {
			return []Instruction{{Op: OpExists, Field: 0}}, nil
		}
		return []Instruction{{Op: OpNotExists, Field: 0}}, nil
	case *interpreterast.NullLiteral:
		return []Instruction{{Op: OpNotExists, Field: 0}}, nil
	case *interpreterast.IntegerLiteral, *interpreterast.FloatLiteral, *interpreterast.StringLiteral:
		if v, ok := literalValue(x); ok && truthy(v) {
			return []Instruction{{Op: OpExists, Field: 0}}, nil
		}
		return []Instruction{{Op: OpNotExists, Field: 0}}, nil
	default:
		return nil, newError(ErrEval, 0, "expression form is not supported by bytecode compiler")
	}
}

func compileCompareExpr(x *interpreterast.InfixExpression, registry *Registry, p *Program) ([]Instruction, error) {
	left, ok := interpreterPath(x.Left)
	if !ok {
		return nil, newError(ErrEval, 0, "bytecode comparisons require a field path on the left")
	}
	field := registry.Field(left)
	op, err := bytecodeOp(x.Operator)
	if err != nil {
		return nil, err
	}
	if x.Operator == "in" || x.Operator == "not in" {
		values, ok := literalArrayValues(x.Right)
		if !ok {
			return nil, newError(ErrEval, 0, "in requires a constant array in bytecode mode")
		}
		setID, kind := p.addSet(registry, values)
		return []Instruction{{Op: op, Field: field, Kind: kind, SetID: setID}}, nil
	}
	value, ok := literalValue(x.Right)
	if !ok {
		return nil, newError(ErrEval, 0, "bytecode comparisons require a literal right-hand value")
	}
	kind, a, err := compileLiteral(registry, value)
	if err != nil {
		return nil, err
	}
	return []Instruction{{Op: op, Field: field, Kind: kind, A: a}}, nil
}

func bytecodeOp(op string) (OpCode, error) {
	switch op {
	case "==":
		return OpEq, nil
	case "!=":
		return OpNeq, nil
	case ">":
		return OpGt, nil
	case ">=":
		return OpGte, nil
	case "<":
		return OpLt, nil
	case "<=":
		return OpLte, nil
	case "in":
		return OpIn, nil
	case "not in":
		return OpNotIn, nil
	default:
		return 0, newError(ErrOperator, 0, "operator %q is not supported by bytecode compiler", op)
	}
}

func compileLiteral(registry *Registry, v any) (ValueKind, int64, error) {
	switch x := v.(type) {
	case nil:
		return ValueMissing, 0, nil
	case bool:
		if x {
			return ValueBool, 1, nil
		}
		return ValueBool, 0, nil
	case string:
		return ValueString, int64(registry.Intern(x)), nil
	case int:
		return ValueInt, int64(x), nil
	case int64:
		return ValueInt, x, nil
	case float64:
		return ValueFloat, int64(math.Float64bits(x)), nil
	case float32:
		return ValueFloat, int64(math.Float64bits(float64(x))), nil
	case time.Time:
		return ValueTime, x.Unix(), nil
	default:
		if f, ok := asFloat(v); ok {
			return ValueFloat, int64(math.Float64bits(f)), nil
		}
		return ValueMissing, 0, newError(ErrType, 0, "unsupported literal %T in bytecode compiler", v)
	}
}

func (p *Program) addSet(registry *Registry, values []any) (uint32, ValueKind) {
	set := compiledSet{}
	for _, v := range values {
		kind, raw, err := compileLiteral(registry, v)
		if err != nil {
			continue
		}
		if set.Kind == ValueMissing {
			set.Kind = kind
		}
		switch kind {
		case ValueInt, ValueTime:
			set.Ints = append(set.Ints, raw)
		case ValueFloat:
			set.Floats = append(set.Floats, math.Float64frombits(uint64(raw)))
		case ValueString:
			set.Strings = append(set.Strings, StringID(raw))
		case ValueBool:
			set.Bools = append(set.Bools, raw != 0)
		}
	}
	sort.Slice(set.Ints, func(i, j int) bool { return set.Ints[i] < set.Ints[j] })
	sort.Slice(set.Floats, func(i, j int) bool { return set.Floats[i] < set.Floats[j] })
	sort.Slice(set.Strings, func(i, j int) bool { return set.Strings[i] < set.Strings[j] })
	id := uint32(len(p.sets))
	p.sets = append(p.sets, set)
	return id, set.Kind
}

func evalInstructions(instructions []Instruction, facts *TypedFacts, ctx *EvalContext, sets []compiledSet) bool {
	if len(instructions) == 0 {
		return true
	}
	stack := ctx.Stack[:0]
	for pc := 0; pc < len(instructions); pc++ {
		ins := instructions[pc]
		switch ins.Op {
		case OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte:
			stack = append(stack, evalCompare(ins, facts))
		case OpIn, OpNotIn:
			ok := evalIn(ins, facts, sets)
			if ins.Op == OpNotIn {
				ok = !ok
			}
			stack = append(stack, ok)
		case OpBetween:
			stack = append(stack, evalBetween(ins, facts))
		case OpExists:
			if ins.Field == 0 {
				stack = append(stack, true)
			} else {
				stack = append(stack, facts != nil && facts.kind(ins.Field) != ValueMissing)
			}
		case OpNotExists:
			if ins.Field == 0 {
				stack = append(stack, false)
			} else {
				stack = append(stack, facts == nil || facts.kind(ins.Field) == ValueMissing)
			}
		case OpAnd:
			n := len(stack)
			v := stack[n-2] && stack[n-1]
			stack = stack[:n-2]
			stack = append(stack, v)
		case OpOr:
			n := len(stack)
			v := stack[n-2] || stack[n-1]
			stack = stack[:n-2]
			stack = append(stack, v)
		case OpNot:
			stack[len(stack)-1] = !stack[len(stack)-1]
		case OpJumpIfFalse:
			if len(stack) > 0 && !stack[len(stack)-1] {
				pc += int(ins.Jump)
			}
		case OpJumpIfTrue:
			if len(stack) > 0 && stack[len(stack)-1] {
				pc += int(ins.Jump)
			}
		}
	}
	ctx.Stack = stack
	return len(stack) > 0 && stack[len(stack)-1]
}

func evalCompare(ins Instruction, facts *TypedFacts) bool {
	if facts == nil || facts.kind(ins.Field) == ValueMissing {
		return ins.Op == OpNeq && ins.Kind == ValueMissing
	}
	cmp, ok := compareTyped(ins, facts)
	if !ok {
		return false
	}
	switch ins.Op {
	case OpEq:
		return cmp == 0
	case OpNeq:
		return cmp != 0
	case OpGt:
		return cmp > 0
	case OpGte:
		return cmp >= 0
	case OpLt:
		return cmp < 0
	case OpLte:
		return cmp <= 0
	default:
		return false
	}
}

func compareTyped(ins Instruction, facts *TypedFacts) (int, bool) {
	kind := facts.kind(ins.Field)
	switch kind {
	case ValueInt:
		right, ok := literalAsInt(ins)
		if !ok {
			return 0, false
		}
		return cmpInt(facts.ints[ins.Field], right), true
	case ValueTime:
		right := ins.A
		return cmpInt(facts.times[ins.Field], right), true
	case ValueFloat:
		right, ok := literalAsFloat(ins)
		if !ok {
			return 0, false
		}
		return cmpFloat(facts.floats[ins.Field], right), true
	case ValueString:
		if ins.Kind != ValueString {
			return 0, false
		}
		return cmpInt(int64(facts.strings[ins.Field]), ins.A), true
	case ValueBool:
		if ins.Kind != ValueBool {
			return 0, false
		}
		left := int64(0)
		if facts.bools[ins.Field] {
			left = 1
		}
		return cmpInt(left, ins.A), true
	default:
		return 0, false
	}
}

func literalAsInt(ins Instruction) (int64, bool) {
	switch ins.Kind {
	case ValueInt, ValueTime:
		return ins.A, true
	case ValueFloat:
		return int64(math.Float64frombits(uint64(ins.A))), true
	default:
		return 0, false
	}
}

func literalAsFloat(ins Instruction) (float64, bool) {
	switch ins.Kind {
	case ValueFloat:
		return math.Float64frombits(uint64(ins.A)), true
	case ValueInt, ValueTime:
		return float64(ins.A), true
	default:
		return 0, false
	}
}

func evalBetween(ins Instruction, facts *TypedFacts) bool {
	if facts == nil || facts.kind(ins.Field) == ValueMissing {
		return false
	}
	min := math.Float64frombits(uint64(ins.A))
	max := math.Float64frombits(uint64(ins.B))
	switch facts.kind(ins.Field) {
	case ValueInt:
		v := float64(facts.ints[ins.Field])
		return v >= min && v <= max
	case ValueFloat:
		v := facts.floats[ins.Field]
		return v >= min && v <= max
	case ValueTime:
		v := float64(facts.times[ins.Field])
		return v >= min && v <= max
	default:
		return false
	}
}

func evalIn(ins Instruction, facts *TypedFacts, sets []compiledSet) bool {
	if facts == nil || int(ins.SetID) >= len(sets) {
		return false
	}
	set := sets[ins.SetID]
	switch facts.kind(ins.Field) {
	case ValueInt:
		v := facts.ints[ins.Field]
		i := sort.Search(len(set.Ints), func(i int) bool { return set.Ints[i] >= v })
		return i < len(set.Ints) && set.Ints[i] == v
	case ValueFloat:
		v := facts.floats[ins.Field]
		i := sort.Search(len(set.Floats), func(i int) bool { return set.Floats[i] >= v })
		return i < len(set.Floats) && set.Floats[i] == v
	case ValueString:
		v := facts.strings[ins.Field]
		i := sort.Search(len(set.Strings), func(i int) bool { return set.Strings[i] >= v })
		return i < len(set.Strings) && set.Strings[i] == v
	case ValueBool:
		for _, v := range set.Bools {
			if v == facts.bools[ins.Field] {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func compileActions(registry *Registry, rule Rule) []CompiledAction {
	actions := make([]CompiledAction, 0, len(rule.Actions)+1)
	switch rule.Decision {
	case "allow":
		actions = append(actions, CompiledAction{Type: ActionAllow})
	case "deny":
		actions = append(actions, CompiledAction{Type: ActionDeny})
	}
	if rule.Score != 0 {
		actions = append(actions, CompiledAction{Type: ActionScore, Value: int64(rule.Score)})
	}
	for _, action := range rule.Actions {
		ca := CompiledAction{Type: registry.actionType(action.Type)}
		for k, v := range action.Payload {
			if ca.Key == 0 {
				ca.Key = registry.actionRef(k)
			}
			switch x := v.(type) {
			case string:
				ca.Ref = registry.actionRef(x)
			case int:
				ca.Value = int64(x)
			case int64:
				ca.Value = x
			case float64:
				ca.Value = int64(x)
			case bool:
				if x {
					ca.Value = 1
				}
			}
		}
		actions = append(actions, ca)
	}
	return actions
}

func countSpecificity(n interpreterast.Expression) int {
	switch x := n.(type) {
	case *interpreterast.InfixExpression:
		if _, ok := interpreterPath(x.Left); ok && x.Operator != "&&" && x.Operator != "||" {
			return 1
		}
		return countSpecificity(x.Left) + countSpecificity(x.Right)
	case *interpreterast.PrefixExpression:
		return countSpecificity(x.Right)
	case *interpreterast.CallExpression:
		name, _ := callName(x)
		if strings.EqualFold(name, "exists") {
			return 1
		}
		total := 0
		for _, arg := range x.Arguments {
			total += countSpecificity(arg)
		}
		return total
	default:
		return 0
	}
}

func firstEqualityIndex(n interpreterast.Expression, registry *Registry) (indexKey, bool) {
	switch x := n.(type) {
	case *interpreterast.InfixExpression:
		if x.Operator == "&&" || x.Operator == "||" {
			if key, ok := firstEqualityIndex(x.Left, registry); ok {
				return key, true
			}
			return firstEqualityIndex(x.Right, registry)
		}
		if x.Operator != "==" {
			return indexKey{}, false
		}
		left, ok := interpreterPath(x.Left)
		if !ok {
			return indexKey{}, false
		}
		right, ok := literalValue(x.Right)
		if !ok {
			return indexKey{}, false
		}
		kind, raw, err := compileLiteral(registry, right)
		if err != nil || kind == ValueMissing {
			return indexKey{}, false
		}
		return indexKey{Field: registry.Field(left), Kind: kind, A: raw}, true
	case *interpreterast.PrefixExpression:
		return indexKey{}, false
	default:
		return indexKey{}, false
	}
}

func literalNumber(n interpreterast.Expression) (float64, bool) {
	v, ok := literalValue(n)
	if !ok {
		return 0, false
	}
	return asFloat(v)
}

func literalValue(n interpreterast.Expression) (any, bool) {
	switch x := n.(type) {
	case *interpreterast.IntegerLiteral:
		return x.Value, true
	case *interpreterast.FloatLiteral:
		return x.Value, true
	case *interpreterast.StringLiteral:
		return x.Value, true
	case *interpreterast.BooleanLiteral:
		return x.Value, true
	case *interpreterast.NullLiteral:
		return nil, true
	default:
		return nil, false
	}
}

func literalArrayValues(n interpreterast.Expression) ([]any, bool) {
	arr, ok := n.(*interpreterast.ArrayLiteral)
	if !ok {
		return nil, false
	}
	out := make([]any, len(arr.Elements))
	for i, item := range arr.Elements {
		v, ok := literalValue(item)
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func callName(c *interpreterast.CallExpression) (string, bool) {
	if c == nil {
		return "", false
	}
	id, ok := c.Function.(*interpreterast.Identifier)
	if !ok {
		return "", false
	}
	return id.Name, true
}

func parseRuleID(id string) RuleID {
	if id == "" {
		return 0
	}
	if n, err := strconv.ParseUint(id, 10, 64); err == nil {
		return RuleID(n)
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return RuleID(h.Sum64())
}

func setTypedValue(registry *Registry, out *TypedFacts, path string, value any) {
	if path == "" {
		return
	}
	id := registry.Field(path)
	switch x := value.(type) {
	case nil:
		out.Grow(id)
	case int:
		out.SetInt(id, int64(x))
	case int8:
		out.SetInt(id, int64(x))
	case int16:
		out.SetInt(id, int64(x))
	case int32:
		out.SetInt(id, int64(x))
	case int64:
		out.SetInt(id, x)
	case uint:
		out.SetInt(id, int64(x))
	case uint8:
		out.SetInt(id, int64(x))
	case uint16:
		out.SetInt(id, int64(x))
	case uint32:
		out.SetInt(id, int64(x))
	case uint64:
		out.SetInt(id, int64(x))
	case float32:
		out.SetFloat(id, float64(x))
	case float64:
		out.SetFloat(id, x)
	case string:
		out.SetString(id, registry.Intern(x))
	case bool:
		out.SetBool(id, x)
	case time.Time:
		out.SetTime(id, x.Unix())
	}
}

func adaptAnyValue(registry *Registry, rv reflect.Value, prefix string, out *TypedFacts) error {
	if !rv.IsValid() {
		return nil
	}
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.CanInterface() {
		if t, ok := rv.Interface().(time.Time); ok {
			setTypedValue(registry, out, prefix, t)
			return nil
		}
	}
	switch rv.Kind() {
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil
		}
		iter := rv.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			path := joinFactPath(prefix, key)
			if err := adaptAnyValue(registry, iter.Value(), path, out); err != nil {
				return err
			}
		}
	case reflect.Struct:
		rt := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			sf := rt.Field(i)
			if sf.PkgPath != "" {
				continue
			}
			name := fieldFactName(sf)
			if name == "-" {
				continue
			}
			path := joinFactPath(prefix, name)
			if err := adaptAnyValue(registry, rv.Field(i), path, out); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		base := prefix
		if base == "" {
			base = "items"
		}
		for i := 0; i < rv.Len(); i++ {
			path := joinFactPath(base, strconv.Itoa(i))
			if err := adaptAnyValue(registry, rv.Index(i), path, out); err != nil {
				return err
			}
		}
	default:
		if rv.CanInterface() {
			setTypedValue(registry, out, prefix, rv.Interface())
		}
	}
	return nil
}

func joinFactPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "." + name
}

func fieldFactName(sf reflect.StructField) string {
	name := sf.Tag.Get("json")
	if comma := strings.IndexByte(name, ','); comma >= 0 {
		name = name[:comma]
	}
	if name == "" {
		name = sf.Tag.Get("condition")
	}
	if name == "" {
		name = strings.ToLower(sf.Name[:1]) + sf.Name[1:]
	}
	return name
}

func copyTypedFacts(dst, src *TypedFacts) {
	if src == nil {
		return
	}
	dst.Reset()
	dst.Grow(FieldID(len(src.kinds)))
	copy(dst.kinds, src.kinds)
	copy(dst.ints, src.ints)
	copy(dst.floats, src.floats)
	copy(dst.strings, src.strings)
	copy(dst.bools, src.bools)
	copy(dst.times, src.times)
}

func growKinds(in []ValueKind, n int) []ValueKind {
	if cap(in) >= n {
		return in[:n]
	}
	out := make([]ValueKind, n)
	copy(out, in)
	return out
}

func growInt64s(in []int64, n int) []int64 {
	if cap(in) >= n {
		return in[:n]
	}
	out := make([]int64, n)
	copy(out, in)
	return out
}

func growFloats(in []float64, n int) []float64 {
	if cap(in) >= n {
		return in[:n]
	}
	out := make([]float64, n)
	copy(out, in)
	return out
}

func growStrings(in []StringID, n int) []StringID {
	if cap(in) >= n {
		return in[:n]
	}
	out := make([]StringID, n)
	copy(out, in)
	return out
}

func growBools(in []bool, n int) []bool {
	if cap(in) >= n {
		return in[:n]
	}
	out := make([]bool, n)
	copy(out, in)
	return out
}

func cmpInt(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func cmpFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
