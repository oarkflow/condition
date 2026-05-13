package condition

import (
	"fmt"
	"math"
	"strings"
	"time"

	interpreterast "github.com/oarkflow/interpreter/pkg/ast"
)

type nativeBool func(facts *rankingFacts, vars Facts, strict bool) (bool, error)
type nativeValue func(facts *rankingFacts, vars Facts, strict bool) (any, error)
type nativePathGetter func(facts *rankingFacts) (any, bool)
type nativeItemPredicate func(item any, facts *rankingFacts, vars Facts, strict bool) (bool, error)

type nativeFilteredCollection struct {
	collection any
	predicate  nativeItemPredicate
	facts      *rankingFacts
	vars       Facts
	strict     bool
}

type nativeBoolCompiler func(*interpreterast.CallExpression) nativeBool
type nativeValueCompiler func(*interpreterast.CallExpression) nativeValue
type nativeAggregateStep func(*nativeAggregateState, float64)
type nativeAggregateResult func(nativeAggregateState) any

type nativeAggregateState struct {
	sum   float64
	count float64
	best  float64
	seen  bool
}

type nativeAggregateSpec struct {
	step   nativeAggregateStep
	result nativeAggregateResult
}

var nativeAggregates = map[string]nativeAggregateSpec{
	"sum": {
		step: func(s *nativeAggregateState, n float64) {
			s.sum += n
			s.count++
		},
		result: func(s nativeAggregateState) any { return s.sum },
	},
	"avg": {
		step: func(s *nativeAggregateState, n float64) {
			s.sum += n
			s.count++
		},
		result: func(s nativeAggregateState) any {
			if s.count == 0 {
				return float64(0)
			}
			return s.sum / s.count
		},
	},
	"min": {
		step: func(s *nativeAggregateState, n float64) {
			if !s.seen || n < s.best {
				s.best = n
				s.seen = true
			}
		},
		result: func(s nativeAggregateState) any {
			if !s.seen {
				return nil
			}
			return s.best
		},
	},
	"max": {
		step: func(s *nativeAggregateState, n float64) {
			if !s.seen || n > s.best {
				s.best = n
				s.seen = true
			}
		},
		result: func(s nativeAggregateState) any {
			if !s.seen {
				return nil
			}
			return s.best
		},
	},
}

var (
	nativeBoolBuiltins  map[string]nativeBoolCompiler
	nativeValueBuiltins map[string]nativeValueCompiler
)

func init() {
	nativeBoolBuiltins = map[string]nativeBoolCompiler{
		"active":          compileNativeActive,
		"nullablematch":   compileNativeNullableMatch,
		"rangematch":      compileNativeRangeMatch,
		"validnow":        compileNativeValidNow,
		"any":             compileNativeValueBool,
		"all":             compileNativeValueBool,
		"none":            compileNativeValueBool,
		"groupcountwhere": compileNativeTruthyValue,
		"groupavgwhere":   compileNativeTruthyValue,
	}

	nativeValueBuiltins = map[string]nativeValueCompiler{
		"now":             compileNativeNow,
		"coalesce":        compileNativeCoalesce,
		"default":         compileNativeDefault,
		"defaultvalue":    compileNativeDefault,
		"count":           compileNativeCount,
		"countwhere":      compileNativeCountWhereCall,
		"sum":             compileNativeNumericAggregateCall(nativeAggregates["sum"]),
		"avg":             compileNativeNumericAggregateCall(nativeAggregates["avg"]),
		"min":             compileNativeNumericAggregateCall(nativeAggregates["min"]),
		"max":             compileNativeNumericAggregateCall(nativeAggregates["max"]),
		"filter":          compileNativeFilter,
		"top":             compileNativeTopBottomCall(true),
		"bottom":          compileNativeTopBottomCall(false),
		"percentile":      compileNativePercentileCall,
		"get":             compileNativeGet,
		"any":             compileNativeQuantifierCall("any"),
		"all":             compileNativeQuantifierCall("all"),
		"none":            compileNativeQuantifierCall("none"),
		"groupcountwhere": compileNativeGroupWhereCall("groupcountwhere"),
		"groupavgwhere":   compileNativeGroupWhereCall("groupavgwhere"),
		"before":          compileNativeBefore,
		"betweentime":     compileNativeBetweenTime,
		"age":             compileNativeAge,
	}
}

func prepareRankNativeRules(rules []compiledRule) {
	for i := range rules {
		if rules[i].expr != nil {
			rules[i].rankNative = compileNativeBool(rules[i].expr.expr)
		}
		if rules[i].group != nil {
			prepareRankNativeGroup(rules[i].group)
		}
	}
}

func prepareRankNativeGroup(group *compiledGroup) {
	if group == nil {
		return
	}
	prepareRankNativeRules(group.rules)
}

func compileNativeBool(n interpreterast.Expression) nativeBool {
	switch x := n.(type) {
	case *interpreterast.InfixExpression:
		switch x.Operator {
		case "&&":
			left := compileNativeBool(x.Left)
			right := compileNativeBool(x.Right)
			if left == nil || right == nil {
				return nil
			}
			return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
				ok, err := left(facts, vars, strict)
				if err != nil || !ok {
					return ok, err
				}
				return right(facts, vars, strict)
			}
		case "||":
			left := compileNativeBool(x.Left)
			right := compileNativeBool(x.Right)
			if left == nil || right == nil {
				return nil
			}
			return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
				ok, err := left(facts, vars, strict)
				if err != nil || ok {
					return ok, err
				}
				return right(facts, vars, strict)
			}
		default:
			if native := compileNativeComparison(x); native != nil {
				return native
			}
			left := compileNativeValue(x.Left)
			right := compileNativeValue(x.Right)
			if left == nil || right == nil {
				return nil
			}
			op := x.Operator
			return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
				l, err := left(facts, vars, strict)
				if err != nil {
					return false, err
				}
				r, err := right(facts, vars, strict)
				if err != nil {
					return false, err
				}
				ok, err := evalNativeOperator(op, nil, l, r)
				if err != nil {
					if strict {
						return false, &Error{Kind: ErrType, Message: err.Error(), Cause: err}
					}
					return false, nil
				}
				return ok, nil
			}
		}
	case *interpreterast.PrefixExpression:
		if x.Operator != "!" {
			return nil
		}
		child := compileNativeBool(x.Right)
		if child != nil {
			return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
				ok, err := child(facts, vars, strict)
				return !ok, err
			}
		}
		val := compileNativeValue(x.Right)
		if val == nil {
			return nil
		}
		return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
			v, err := val(facts, vars, strict)
			return !truthy(v), err
		}
	case *interpreterast.CallExpression:
		if native := compileNativeCallBool(x); native != nil {
			return native
		}
		val := compileNativeValue(x)
		if val == nil {
			return nil
		}
		return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
			v, err := val(facts, vars, strict)
			return truthy(v), err
		}
	default:
		val := compileNativeValue(n)
		if val == nil {
			return nil
		}
		return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
			v, err := val(facts, vars, strict)
			return truthy(v), err
		}
	}
}

func compileNativeCallBool(x *interpreterast.CallExpression) nativeBool {
	rawName, ok := callName(x)
	if !ok {
		return nil
	}
	if compiler := nativeBoolBuiltins[strings.ToLower(rawName)]; compiler != nil {
		return compiler(x)
	}
	return nil
}

func compileNativeValueBool(x *interpreterast.CallExpression) nativeBool {
	value := compileNativeCallValue(x)
	if value == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
		v, err := value(facts, vars, strict)
		if err != nil {
			return false, err
		}
		b, ok := v.(bool)
		return ok && b, nil
	}
}

func compileNativeTruthyValue(x *interpreterast.CallExpression) nativeBool {
	value := compileNativeCallValue(x)
	if value == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
		v, err := value(facts, vars, strict)
		if err != nil {
			return false, err
		}
		return truthy(v), nil
	}
}

func compileNativeActive(x *interpreterast.CallExpression) nativeBool {
	if len(x.Arguments) != 1 {
		return nil
	}
	value := compileNativeValue(x.Arguments[0])
	if value == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
		status, err := value(facts, vars, strict)
		if err != nil {
			return false, err
		}
		s, ok := status.(string)
		return ok && strings.EqualFold(s, "active"), nil
	}
}

func compileNativeNullableMatch(x *interpreterast.CallExpression) nativeBool {
	if len(x.Arguments) != 2 {
		return nil
	}
	routeValue := compileNativeValue(x.Arguments[0])
	inputValue := compileNativeValue(x.Arguments[1])
	if routeValue == nil || inputValue == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
		rv, err := routeValue(facts, vars, strict)
		if err != nil {
			return false, err
		}
		if rv == nil {
			return true, nil
		}
		iv, err := inputValue(facts, vars, strict)
		if err != nil {
			return false, err
		}
		return equal(rv, iv), nil
	}
}

func compileNativeRangeMatch(x *interpreterast.CallExpression) nativeBool {
	if len(x.Arguments) != 3 {
		return nil
	}
	minValue := compileNativeValue(x.Arguments[0])
	maxValue := compileNativeValue(x.Arguments[1])
	inputValue := compileNativeValue(x.Arguments[2])
	if minValue == nil || maxValue == nil || inputValue == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
		value, err := inputValue(facts, vars, strict)
		if err != nil {
			return false, err
		}
		n, ok := asFloat(value)
		if !ok {
			return false, nil
		}
		min, err := minValue(facts, vars, strict)
		if err != nil {
			return false, err
		}
		if min != nil {
			mn, ok := asFloat(min)
			if !ok || n < mn {
				return false, nil
			}
		}
		max, err := maxValue(facts, vars, strict)
		if err != nil {
			return false, err
		}
		if max != nil {
			mx, ok := asFloat(max)
			if !ok || n > mx {
				return false, nil
			}
		}
		return true, nil
	}
}

func compileNativeValidNow(x *interpreterast.CallExpression) nativeBool {
	if len(x.Arguments) != 2 {
		return nil
	}
	fromValue := compileNativeValue(x.Arguments[0])
	toValue := compileNativeValue(x.Arguments[1])
	if fromValue == nil || toValue == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
		from, err := fromValue(facts, vars, strict)
		if err != nil {
			return false, err
		}
		to, err := toValue(facts, vars, strict)
		if err != nil {
			return false, err
		}
		if from == nil && to == nil {
			return true, nil
		}
		now := time.Now()
		if from != nil {
			t, err := parseTimeValue(from)
			if err != nil || now.Before(t) {
				return false, nil
			}
		}
		if to != nil {
			t, err := parseTimeValue(to)
			if err != nil || now.After(t) {
				return false, nil
			}
		}
		return true, nil
	}
}

func compileNativeComparison(x *interpreterast.InfixExpression) nativeBool {
	if variable, ok := x.Left.(*interpreterast.Identifier); ok {
		name := variable.Name
		parts := []string{name}
		lget := compileNativePathGetter(parts)
		if rightPath, ok := interpreterPath(x.Right); ok {
			rightParts := strings.Split(rightPath, ".")
			rget := compileNativePathGetter(rightParts)
			return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
				l, ok := getVariableParts(vars, parts)
				if !ok {
					l, ok = lget(facts)
					if !ok {
						if strict {
							return false, newError(ErrMissing, 0, "missing fact or variable %q", name)
						}
						l = nil
					}
				}
				r, ok := rget(facts)
				if !ok {
					if strict {
						return false, newError(ErrMissing, 0, "missing fact %q", rightPath)
					}
					r = nil
				}
				return evalNativeOperatorStrict(x.Operator, nil, l, r, strict)
			}
		}
	}
	if leftPath, ok := interpreterPath(x.Left); ok {
		leftParts := strings.Split(leftPath, ".")
		lget := compileNativePathGetter(leftParts)
		if variable, ok := x.Right.(*interpreterast.Identifier); ok {
			name := variable.Name
			parts := []string{name}
			rget := compileNativePathGetter(parts)
			return func(facts *rankingFacts, vars Facts, strict bool) (bool, error) {
				l, ok := lget(facts)
				if !ok {
					if strict {
						return false, newError(ErrMissing, 0, "missing fact %q", leftPath)
					}
					l = nil
				}
				r, ok := getVariableParts(vars, parts)
				if !ok {
					r, ok = rget(facts)
					if !ok {
						if strict {
							return false, newError(ErrMissing, 0, "missing fact or variable %q", name)
						}
						r = nil
					}
				}
				return evalNativeOperatorStrict(x.Operator, nil, l, r, strict)
			}
		}
		if rightPath, ok := interpreterPath(x.Right); ok {
			rightParts := strings.Split(rightPath, ".")
			rget := compileNativePathGetter(rightParts)
			return func(facts *rankingFacts, _ Facts, strict bool) (bool, error) {
				l, ok := lget(facts)
				if !ok {
					if strict {
						return false, newError(ErrMissing, 0, "missing fact %q", leftPath)
					}
					l = nil
				}
				r, ok := rget(facts)
				if !ok {
					if strict {
						return false, newError(ErrMissing, 0, "missing fact %q", rightPath)
					}
					r = nil
				}
				return evalNativeOperatorStrict(x.Operator, nil, l, r, strict)
			}
		}
		if r, ok := literalValue(x.Right); ok {
			return func(facts *rankingFacts, _ Facts, strict bool) (bool, error) {
				l, ok := lget(facts)
				if !ok {
					if strict {
						return false, newError(ErrMissing, 0, "missing fact %q", leftPath)
					}
					l = nil
				}
				return evalNativeOperatorStrict(x.Operator, nil, l, r, strict)
			}
		}
	}
	if l, ok := literalValue(x.Left); ok {
		if rightPath, ok := interpreterPath(x.Right); ok {
			rightParts := strings.Split(rightPath, ".")
			rget := compileNativePathGetter(rightParts)
			return func(facts *rankingFacts, _ Facts, strict bool) (bool, error) {
				r, ok := rget(facts)
				if !ok {
					if strict {
						return false, newError(ErrMissing, 0, "missing fact %q", rightPath)
					}
					r = nil
				}
				return evalNativeOperatorStrict(x.Operator, nil, l, r, strict)
			}
		}
	}
	return nil
}

func compileNativeValue(n interpreterast.Expression) nativeValue {
	switch x := n.(type) {
	case *interpreterast.IntegerLiteral, *interpreterast.FloatLiteral, *interpreterast.StringLiteral, *interpreterast.BooleanLiteral, *interpreterast.NullLiteral:
		v, _ := literalValue(x)
		return func(*rankingFacts, Facts, bool) (any, error) { return v, nil }
	case *interpreterast.TernaryExpression:
		cond := compileNativeBool(x.Condition)
		thenValue := compileNativeValue(x.Consequence)
		elseValue := compileNativeValue(x.Alternative)
		if cond == nil || thenValue == nil || elseValue == nil {
			return nil
		}
		return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
			ok, err := cond(facts, vars, strict)
			if err != nil {
				return nil, err
			}
			if ok {
				return thenValue(facts, vars, strict)
			}
			return elseValue(facts, vars, strict)
		}
	case *interpreterast.CallExpression:
		return compileNativeCallValue(x)
	case *interpreterast.Identifier, *interpreterast.DotExpression, *interpreterast.IndexExpression:
		path, ok := interpreterPath(x)
		if !ok {
			return nil
		}
		parts := strings.Split(path, ".")
		get := compileNativePathGetter(parts)
		return func(facts *rankingFacts, _ Facts, strict bool) (any, error) {
			v, ok := get(facts)
			if ok {
				return v, nil
			}
			if strict {
				return nil, newError(ErrMissing, 0, "missing fact %q", path)
			}
			return nil, nil
		}
	case *interpreterast.ArrayLiteral:
		values, ok := literalArrayValues(x)
		if !ok {
			return nil
		}
		return func(*rankingFacts, Facts, bool) (any, error) { return values, nil }
	default:
		return nil
	}
}

func compileNativeCallValue(x *interpreterast.CallExpression) nativeValue {
	rawName, ok := callName(x)
	if !ok {
		return nil
	}
	if compiler := nativeValueBuiltins[strings.ToLower(rawName)]; compiler != nil {
		return compiler(x)
	}
	return nil
}

func compileNativeNow(x *interpreterast.CallExpression) nativeValue {
	if len(x.Arguments) != 0 {
		return nil
	}
	return func(*rankingFacts, Facts, bool) (any, error) { return time.Now().UTC(), nil }
}

func compileNativeCoalesce(x *interpreterast.CallExpression) nativeValue {
	values := compileNativeValues(x.Arguments)
	if len(values) != len(x.Arguments) {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		for _, value := range values {
			v, err := value(facts, vars, strict)
			if err != nil {
				return nil, err
			}
			if !isEmpty(v) {
				return v, nil
			}
		}
		return nil, nil
	}
}

func compileNativeDefault(x *interpreterast.CallExpression) nativeValue {
	if len(x.Arguments) != 2 {
		return nil
	}
	value := compileNativeValue(x.Arguments[0])
	fallback := compileNativeValue(x.Arguments[1])
	if value == nil || fallback == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		v, err := value(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		if !isEmpty(v) {
			return v, nil
		}
		return fallback(facts, vars, strict)
	}
}

func compileNativeCount(x *interpreterast.CallExpression) nativeValue {
	if len(x.Arguments) != 1 {
		return compileNativeCountWhere(x.Arguments)
	}
	collection := compileNativeValue(x.Arguments[0])
	if collection == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		c, err := collection(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		return float64(nativeCollectionLen(c)), nil
	}
}

func compileNativeCountWhereCall(x *interpreterast.CallExpression) nativeValue {
	return compileNativeCountWhere(x.Arguments)
}

func compileNativeNumericAggregateCall(spec nativeAggregateSpec) nativeValueCompiler {
	return func(x *interpreterast.CallExpression) nativeValue {
		return compileNativeNumericAggregate(spec, x.Arguments)
	}
}

func compileNativeFilter(x *interpreterast.CallExpression) nativeValue {
	if len(x.Arguments) < 2 || len(x.Arguments) > 4 {
		return nil
	}
	collection := compileNativeValue(x.Arguments[0])
	predicate := compileNativeItemPredicate(x.Arguments[1:])
	if collection == nil || predicate == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		c, err := collection(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		return nativeFilteredCollection{collection: c, predicate: predicate, facts: facts, vars: vars, strict: strict}, nil
	}
}

func compileNativeTopBottomCall(top bool) nativeValueCompiler {
	return func(x *interpreterast.CallExpression) nativeValue {
		if len(x.Arguments) != 2 {
			return nil
		}
		collection := compileNativeValue(x.Arguments[0])
		field, ok := literalString(x.Arguments[1])
		if collection == nil || !ok {
			return nil
		}
		parts := strings.Split(field, ".")
		return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
			c, err := collection(facts, vars, strict)
			if err != nil {
				return nil, err
			}
			return nativeTopBottom(c, parts, top)
		}
	}
}

func compileNativePercentileCall(x *interpreterast.CallExpression) nativeValue {
	if len(x.Arguments) != 3 {
		return nil
	}
	collection := compileNativeValue(x.Arguments[0])
	field, ok := literalString(x.Arguments[1])
	pvalue := compileNativeValue(x.Arguments[2])
	if collection == nil || !ok || pvalue == nil {
		return nil
	}
	parts := strings.Split(field, ".")
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		c, err := collection(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		p, err := pvalue(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		return nativePercentile(c, parts, p)
	}
}

func compileNativeGet(x *interpreterast.CallExpression) nativeValue {
	if len(x.Arguments) != 2 {
		return nil
	}
	value := compileNativeValue(x.Arguments[0])
	path, ok := literalString(x.Arguments[1])
	if value == nil || !ok {
		return nil
	}
	parts := strings.Split(path, ".")
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		v, err := value(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		out, _ := lookupParts(v, parts)
		return out, nil
	}
}

func compileNativeQuantifierCall(mode string) nativeValueCompiler {
	return func(x *interpreterast.CallExpression) nativeValue {
		if len(x.Arguments) < 2 || len(x.Arguments) > 4 {
			return nil
		}
		collection := compileNativeValue(x.Arguments[0])
		predicate := compileNativeItemPredicate(x.Arguments[1:])
		if collection == nil || predicate == nil {
			return nil
		}
		return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
			c, err := collection(facts, vars, strict)
			if err != nil {
				return nil, err
			}
			return nativeAnyAllNone(c, predicate, facts, vars, strict, mode)
		}
	}
}

func compileNativeGroupWhereCall(name string) nativeValueCompiler {
	return func(x *interpreterast.CallExpression) nativeValue {
		return compileNativeGroupWhere(name, x.Arguments)
	}
}

func compileNativeBefore(x *interpreterast.CallExpression) nativeValue {
	if len(x.Arguments) != 2 {
		return nil
	}
	left := compileNativeValue(x.Arguments[0])
	right := compileNativeValue(x.Arguments[1])
	if left == nil || right == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		lv, err := left(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		rv, err := right(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		lt, err := parseTimeValue(lv)
		if err != nil {
			return nil, err
		}
		rt, err := parseTimeValue(rv)
		if err != nil {
			return nil, err
		}
		return lt.Before(rt), nil
	}
}

func compileNativeBetweenTime(x *interpreterast.CallExpression) nativeValue {
	if len(x.Arguments) != 3 {
		return nil
	}
	values := compileNativeValues(x.Arguments)
	if len(values) != 3 {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		v, err := nativeTime(values[0], facts, vars, strict)
		if err != nil {
			return nil, err
		}
		start, err := nativeTime(values[1], facts, vars, strict)
		if err != nil {
			return nil, err
		}
		end, err := nativeTime(values[2], facts, vars, strict)
		if err != nil {
			return nil, err
		}
		return (v.Equal(start) || v.After(start)) && (v.Equal(end) || v.Before(end)), nil
	}
}

func compileNativeAge(x *interpreterast.CallExpression) nativeValue {
	if len(x.Arguments) < 1 || len(x.Arguments) > 2 {
		return nil
	}
	value := compileNativeValue(x.Arguments[0])
	if value == nil {
		return nil
	}
	unit := "seconds"
	if len(x.Arguments) == 2 {
		var ok bool
		unit, ok = literalString(x.Arguments[1])
		if !ok {
			return nil
		}
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		t, err := nativeTime(value, facts, vars, strict)
		if err != nil {
			return nil, err
		}
		seconds := time.Since(t).Seconds()
		switch strings.ToLower(unit) {
		case "second", "seconds":
			return seconds, nil
		case "minute", "minutes":
			return seconds / 60, nil
		case "hour", "hours":
			return seconds / 3600, nil
		case "day", "days":
			return seconds / 86400, nil
		default:
			return nil, fmt.Errorf("unsupported age unit %q", unit)
		}
	}
}

func compileNativeValues(args []interpreterast.Expression) []nativeValue {
	values := make([]nativeValue, 0, len(args))
	for _, arg := range args {
		value := compileNativeValue(arg)
		if value == nil {
			return values
		}
		values = append(values, value)
	}
	return values
}

func compileNativeCountWhere(args []interpreterast.Expression) nativeValue {
	if len(args) < 2 || len(args) > 4 {
		return nil
	}
	collection := compileNativeValue(args[0])
	predicate := compileNativeItemPredicate(args[1:])
	if collection == nil || predicate == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		c, err := collection(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		count := 0
		err = nativeEachCollection(c, func(item any) error {
			ok, err := predicate(item, facts, vars, strict)
			if err != nil {
				return err
			}
			if ok {
				count++
			}
			return nil
		})
		return float64(count), err
	}
}

func compileNativeNumericAggregate(spec nativeAggregateSpec, args []interpreterast.Expression) nativeValue {
	if len(args) < 1 || len(args) > 2 {
		return nil
	}
	collection := compileNativeValue(args[0])
	var parts []string
	if len(args) == 2 {
		field, ok := literalString(args[1])
		if !ok {
			return nil
		}
		parts = strings.Split(field, ".")
	}
	if collection == nil {
		return nil
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		c, err := collection(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		state := nativeAggregateState{}
		err = nativeEachCollection(c, func(item any) error {
			v := item
			if len(parts) != 0 {
				var ok bool
				v, ok = lookupParts(item, parts)
				if !ok || v == nil {
					return nil
				}
			}
			n, ok := asFloat(v)
			if !ok {
				return fmt.Errorf("aggregation value %v (%T) is not numeric", v, v)
			}
			spec.step(&state, n)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return spec.result(state), nil
	}
}

func compileNativeGroupWhere(name string, args []interpreterast.Expression) nativeValue {
	if len(args) != 6 && len(args) != 7 {
		return nil
	}
	if name == "groupcountwhere" && len(args) != 6 {
		return nil
	}
	if name == "groupavgwhere" && len(args) != 7 {
		return nil
	}
	collection := compileNativeValue(args[0])
	groupField, ok := literalString(args[1])
	if collection == nil || !ok {
		return nil
	}
	groupValue := compileNativeValue(args[2])
	predicate := compileNativeItemPredicate(args[3:6])
	if groupValue == nil || predicate == nil {
		return nil
	}
	groupParts := strings.Split(groupField, ".")
	var valueParts []string
	if name == "groupavgwhere" {
		valueField, ok := literalString(args[6])
		if !ok {
			return nil
		}
		valueParts = strings.Split(valueField, ".")
	}
	return func(facts *rankingFacts, vars Facts, strict bool) (any, error) {
		c, err := collection(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		gv, err := groupValue(facts, vars, strict)
		if err != nil {
			return nil, err
		}
		count := 0
		sum := 0.0
		err = nativeEachCollection(c, func(item any) error {
			key, ok := lookupParts(item, groupParts)
			if !ok || !equal(key, gv) {
				return nil
			}
			ok, err := predicate(item, facts, vars, strict)
			if err != nil || !ok {
				return err
			}
			count++
			if name == "groupavgwhere" {
				v, ok := lookupParts(item, valueParts)
				if !ok || v == nil {
					return nil
				}
				n, ok := asFloat(v)
				if !ok {
					return fmt.Errorf("groupAvgWhere value %v (%T) is not numeric", v, v)
				}
				sum += n
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if name == "groupavgwhere" {
			if count == 0 {
				return float64(0), nil
			}
			return sum / float64(count), nil
		}
		return float64(count), nil
	}
}

func compileNativeItemPredicate(args []interpreterast.Expression) nativeItemPredicate {
	if len(args) < 1 || len(args) > 3 {
		return nil
	}
	field, ok := literalString(args[0])
	if !ok {
		return nil
	}
	parts := strings.Split(field, ".")
	if len(args) == 1 {
		return func(item any, _ *rankingFacts, _ Facts, _ bool) (bool, error) {
			v, _ := lookupParts(item, parts)
			return truthy(v), nil
		}
	}
	var op string
	valueIndex := 1
	if len(args) == 3 {
		var ok bool
		op, ok = literalString(args[1])
		if !ok {
			return nil
		}
		valueIndex = 2
	}
	value := compileNativeValue(args[valueIndex])
	if value == nil {
		return nil
	}
	return func(item any, facts *rankingFacts, vars Facts, strict bool) (bool, error) {
		left, _ := lookupParts(item, parts)
		right, err := value(facts, vars, strict)
		if err != nil {
			return false, err
		}
		if len(args) == 2 {
			return equal(left, right), nil
		}
		return evalNativeOperatorStrict(op, nil, left, right, strict)
	}
}

func literalString(n interpreterast.Expression) (string, bool) {
	v, ok := literalValue(n)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func nativeTime(value nativeValue, facts *rankingFacts, vars Facts, strict bool) (time.Time, error) {
	v, err := value(facts, vars, strict)
	if err != nil {
		return time.Time{}, err
	}
	return parseTimeValue(v)
}

func nativeAnyAllNone(collection any, predicate nativeItemPredicate, facts *rankingFacts, vars Facts, strict bool, mode string) (bool, error) {
	seen := false
	matched := mode == "all"
	err := nativeEachCollection(collection, func(item any) error {
		if mode == "any" && matched {
			return nil
		}
		if mode == "all" && seen && !matched {
			return nil
		}
		seen = true
		ok, err := predicate(item, facts, vars, strict)
		if err != nil {
			return err
		}
		switch mode {
		case "any":
			matched = ok
		case "all":
			matched = ok
		case "none":
			if ok {
				matched = true
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	switch mode {
	case "all":
		return seen && matched, nil
	case "none":
		return !matched, nil
	default:
		return matched, nil
	}
}

func nativeTopBottom(collection any, parts []string, top bool) (any, error) {
	var out any
	best := 0.0
	seen := false
	err := nativeEachCollection(collection, func(item any) error {
		v, ok := lookupParts(item, parts)
		if !ok || v == nil {
			return nil
		}
		n, ok := asFloat(v)
		if !ok {
			return fmt.Errorf("top/bottom value %v (%T) is not numeric", v, v)
		}
		if !seen || (top && n > best) || (!top && n < best) {
			best = n
			out = item
			seen = true
		}
		return nil
	})
	return out, err
}

func nativePercentile(collection any, parts []string, percentile any) (any, error) {
	p, ok := asFloat(percentile)
	if !ok {
		return nil, fmt.Errorf("percentile p must be numeric")
	}
	if p > 1 {
		p = p / 100
	}
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	values := make([]float64, 0, nativeCollectionLen(collection))
	err := nativeEachCollection(collection, func(item any) error {
		v, ok := lookupParts(item, parts)
		if !ok || v == nil {
			return nil
		}
		n, ok := asFloat(v)
		if !ok {
			return fmt.Errorf("percentile value %v (%T) is not numeric", v, v)
		}
		values = append(values, n)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	sortFloat64s(values)
	return values[int(math.Round(p*float64(len(values)-1)))], nil
}

func nativeCollectionLen(v any) int {
	if filtered, ok := v.(nativeFilteredCollection); ok {
		count := 0
		_ = nativeEachCollection(filtered.collection, func(item any) error {
			ok, err := filtered.predicate(item, filtered.facts, filtered.vars, filtered.strict)
			if err == nil && ok {
				count++
			}
			return err
		})
		return count
	}
	return collectionLen(v)
}

func nativeEachCollection(v any, fn func(any) error) error {
	if filtered, ok := v.(nativeFilteredCollection); ok {
		return nativeEachCollection(filtered.collection, func(item any) error {
			ok, err := filtered.predicate(item, filtered.facts, filtered.vars, filtered.strict)
			if err != nil || !ok {
				return err
			}
			return fn(item)
		})
	}
	switch x := v.(type) {
	case []map[string]any:
		for _, item := range x {
			if err := fn(item); err != nil {
				return err
			}
		}
		return nil
	case []MapFacts:
		for _, item := range x {
			if err := fn(item); err != nil {
				return err
			}
		}
		return nil
	default:
		return eachCollection(v, fn)
	}
}

func evalNativeOperator(op string, fn Operator, l, r any) (bool, error) {
	switch op {
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
		if ok, handled := nativeContains(r, l); handled {
			return ok, nil
		}
		return contains(r, l)
	case "not in":
		if ok, handled := nativeContains(r, l); handled {
			return !ok, nil
		}
		ok, err := contains(r, l)
		return !ok, err
	case "contains":
		if ok, handled := nativeContains(l, r); handled {
			return ok, nil
		}
		return contains(l, r)
	default:
		if fn == nil {
			return false, newError(ErrOperator, 0, "unknown operator %q", op)
		}
		return fn(l, r)
	}
}

func nativeContains(container, item any) (bool, bool) {
	switch c := container.(type) {
	case []string:
		it, ok := item.(string)
		if !ok {
			return false, true
		}
		for _, v := range c {
			if v == it {
				return true, true
			}
		}
		return false, true
	case []int:
		it, ok := item.(int)
		if ok {
			for _, v := range c {
				if v == it {
					return true, true
				}
			}
			return false, true
		}
	case []float64:
		it, ok := item.(float64)
		if ok {
			for _, v := range c {
				if v == it {
					return true, true
				}
			}
			return false, true
		}
	case map[string]any:
		it, ok := item.(string)
		if !ok {
			return false, true
		}
		_, ok = c[it]
		return ok, true
	case MapFacts:
		it, ok := item.(string)
		if !ok {
			return false, true
		}
		_, ok = c[it]
		return ok, true
	case string:
		it, ok := item.(string)
		if !ok {
			return false, true
		}
		return strings.Contains(c, it), true
	}
	return false, false
}

func evalNativeOperatorStrict(op string, fn Operator, l, r any, strict bool) (bool, error) {
	ok, err := evalNativeOperator(op, fn, l, r)
	if err != nil {
		if strict {
			return false, &Error{Kind: ErrType, Message: err.Error(), Cause: err}
		}
		return false, nil
	}
	return ok, nil
}

func getNativePath(f *rankingFacts, parts []string) (any, bool) {
	if f == nil || len(parts) == 0 {
		return nil, false
	}
	return f.GetPath(parts)
}

func compileNativePathGetter(parts []string) nativePathGetter {
	if len(parts) == 0 {
		return nil
	}
	first := parts[0]
	if len(parts) == 2 {
		second := parts[1]
		switch first {
		case "provider", "quality":
			return func(f *rankingFacts) (any, bool) {
				if f == nil || f.provider == nil {
					return nil, false
				}
				if m, ok := f.provider.(MapFacts); ok {
					if root, hasRoot := m[first]; hasRoot {
						return lookupParts(root, parts[1:])
					}
					v, ok := m[second]
					return v, ok
				}
				return f.domainValue(first, parts[1:], false)
			}
		case "route", "candidate":
			return func(f *rankingFacts) (any, bool) {
				if f == nil || f.provider == nil {
					return nil, false
				}
				if m, ok := f.provider.(MapFacts); ok {
					if root, hasRoot := m[first]; hasRoot {
						return lookupParts(root, parts[1:])
					}
					v, ok := m[second]
					return v, ok
				}
				return f.domainValue(first, parts[1:], true)
			}
		default:
			return func(f *rankingFacts) (any, bool) {
				if f == nil || f.global == nil {
					return nil, false
				}
				if m, ok := f.global.(MapFacts); ok {
					return lookupMap2(map[string]any(m), first, second)
				}
				return getPathParts(f.global, parts)
			}
		}
	}
	return func(f *rankingFacts) (any, bool) { return getNativePath(f, parts) }
}

func lookupMap2(m map[string]any, first, second string) (any, bool) {
	root, ok := m[first]
	if !ok {
		return nil, false
	}
	if mm, ok := root.(map[string]any); ok {
		v, ok := mm[second]
		return v, ok
	}
	if mm, ok := root.(MapFacts); ok {
		v, ok := mm[second]
		return v, ok
	}
	return lookupParts(root, []string{second})
}

func nativeFloat(get nativePathGetter, facts *rankingFacts) (float64, bool) {
	if get == nil {
		return 0, false
	}
	v, ok := get(facts)
	if !ok {
		return 0, false
	}
	return asFloat(v)
}
