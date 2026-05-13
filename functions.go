package condition

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
)

type Function func(ctx EvalContext, args ...any) (any, error)

type EvalContext struct {
	Context    contextLike
	Facts      Facts
	Matched    []RuleID
	Actions    []CompiledAction
	Stack      []bool
	Trace      []TraceStep
	Candidates []int
	TypedFacts *TypedFacts
}

type contextLike interface {
	Done() <-chan struct{}
	Err() error
}

type FunctionRegistry struct {
	mu sync.RWMutex
	m  map[string]Function
}

func NewFunctionRegistry() *FunctionRegistry {
	r := &FunctionRegistry{m: make(map[string]Function)}
	return r
}

func (r *FunctionRegistry) Register(name string, fn Function) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[strings.ToLower(name)] = fn
	r.m[name] = fn
}

func (r *FunctionRegistry) Get(name string) (Function, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.m[strings.ToLower(name)]
	return fn, ok
}

var globalFunctions = func() *FunctionRegistry {
	r := NewFunctionRegistry()
	r.Register("len", func(_ EvalContext, args ...any) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("len expects 1 argument")
		}
		v := reflect.ValueOf(args[0])
		if !v.IsValid() {
			return float64(0), nil
		}
		switch v.Kind() {
		case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
			return float64(v.Len()), nil
		default:
			return nil, fmt.Errorf("len unsupported for %T", args[0])
		}
	})
	r.Register("empty", func(_ EvalContext, args ...any) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("empty expects 1 argument")
		}
		return isEmpty(args[0]), nil
	})
	r.Register("exists", func(ctx EvalContext, args ...any) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("exists expects 1 argument")
		}
		p, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("exists expects path string")
		}
		_, found := ctx.Facts.Get(p)
		return found, nil
	})
	registerDSLHelperFunctions(r)
	registerAggregateFunctions(r)
	return r
}()

func GlobalFunctions() *FunctionRegistry { return globalFunctions }

func RegisterFunction(name string, fn Function) { globalFunctions.Register(name, fn) }

type Operator func(left, right any) (bool, error)

type OperatorRegistry struct {
	mu sync.RWMutex
	m  map[string]Operator
}

func NewOperatorRegistry() *OperatorRegistry {
	return &OperatorRegistry{m: make(map[string]Operator)}
}

func (r *OperatorRegistry) Register(name string, op Operator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[strings.ToLower(name)] = op
}

func (r *OperatorRegistry) Get(name string) (Operator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	op, ok := r.m[strings.ToLower(name)]
	return op, ok
}

var globalOperators = func() *OperatorRegistry {
	r := NewOperatorRegistry()
	for _, op := range []string{"==", "!=", "<", "<=", ">", ">=", "in", "not in", "contains", "matches", "startswith", "endswith"} {
		name := op
		r.Register(name, func(l, rr any) (bool, error) { return evalOperator(name, l, rr) })
	}
	return r
}()

func GlobalOperators() *OperatorRegistry { return globalOperators }

func RegisterOperator(name string, op Operator) { globalOperators.Register(name, op) }

var regexCache sync.Map

func evalOperator(op string, l, r any) (bool, error) {
	switch strings.ToLower(op) {
	case "==":
		return equal(l, r), nil
	case "!=":
		return !equal(l, r), nil
	case "<", "<=", ">", ">=":
		c, ok := compare(l, r)
		if !ok {
			return false, fmt.Errorf("cannot compare %T and %T", l, r)
		}
		switch op {
		case "<":
			return c < 0, nil
		case "<=":
			return c <= 0, nil
		case ">":
			return c > 0, nil
		default:
			return c >= 0, nil
		}
	case "in":
		return contains(r, l)
	case "not in":
		ok, err := contains(r, l)
		return !ok, err
	case "contains":
		return contains(l, r)
	case "matches":
		ls, ok1 := asString(l)
		rs, ok2 := asString(r)
		if !ok1 || !ok2 {
			return false, fmt.Errorf("matches expects strings")
		}
		raw, ok := regexCache.Load(rs)
		if !ok {
			compiled, err := regexp.Compile(rs)
			if err != nil {
				return false, err
			}
			raw, _ = regexCache.LoadOrStore(rs, compiled)
		}
		return raw.(*regexp.Regexp).MatchString(ls), nil
	case "startswith":
		ls, ok1 := asString(l)
		rs, ok2 := asString(r)
		return ok1 && ok2 && strings.HasPrefix(ls, rs), nil
	case "endswith":
		ls, ok1 := asString(l)
		rs, ok2 := asString(r)
		return ok1 && ok2 && strings.HasSuffix(ls, rs), nil
	default:
		return false, newError(ErrOperator, 0, "unknown operator %q", op)
	}
}
