package condition

import (
	"fmt"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

var parsedTimeCache sync.Map

func registerDSLHelperFunctions(r *FunctionRegistry) {
	r.Register("if", fnIf)
	r.Register("coalesce", fnCoalesce)
	r.Register("default", fnDefault)
	r.Register("isNull", fnIsNull)
	r.Register("isNotNull", fnIsNotNull)
	r.Register("lower", fnLower)
	r.Register("upper", fnUpper)
	r.Register("trim", fnTrim)
	r.Register("hasPrefix", fnHasPrefix)
	r.Register("hasSuffix", fnHasSuffix)
	r.Register("split", fnSplit)
	r.Register("join", fnJoin)
	r.Register("abs", fnAbs)
	r.Register("round", fnRound)
	r.Register("floor", fnFloor)
	r.Register("ceil", fnCeil)
	r.Register("clamp", fnClamp)
	r.Register("between", fnBetween)
	r.Register("now", fnNow)
	r.Register("date", fnDate)
	r.Register("before", fnBefore)
	r.Register("after", fnAfter)
	r.Register("betweenTime", fnBetweenTime)
	r.Register("age", fnAge)
	r.Register("nullableMatch", fnNullableMatch)
	r.Register("rangeMatch", fnRangeMatch)
	r.Register("active", fnActive)
	r.Register("validNow", fnValidNow)
	r.Register("specificity", fnSpecificity)
	r.Register("stableBucket", fnStableBucket)
	r.Register("get", fnGet)
}

func fnIf(_ EvalContext, args ...any) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("if expects condition, then, else")
	}
	if truthy(args[0]) {
		return args[1], nil
	}
	return args[2], nil
}

func fnCoalesce(_ EvalContext, args ...any) (any, error) {
	for _, arg := range args {
		if !isEmpty(arg) {
			return arg, nil
		}
	}
	return nil, nil
}

func fnDefault(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("default expects value and fallback")
	}
	if isEmpty(args[0]) {
		return args[1], nil
	}
	return args[0], nil
}

func fnIsNull(_ EvalContext, args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("isNull expects 1 argument")
	}
	return args[0] == nil, nil
}

func fnIsNotNull(ctx EvalContext, args ...any) (any, error) {
	v, err := fnIsNull(ctx, args...)
	return !truthy(v), err
}

func fnLower(_ EvalContext, args ...any) (any, error) {
	s, err := oneString("lower", args)
	return strings.ToLower(s), err
}

func fnUpper(_ EvalContext, args ...any) (any, error) {
	s, err := oneString("upper", args)
	return strings.ToUpper(s), err
}

func fnTrim(_ EvalContext, args ...any) (any, error) {
	s, err := oneString("trim", args)
	return strings.TrimSpace(s), err
}

func fnHasPrefix(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("hasPrefix expects value and prefix")
	}
	s, ok1 := asString(args[0])
	p, ok2 := asString(args[1])
	return ok1 && ok2 && strings.HasPrefix(s, p), nil
}

func fnHasSuffix(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("hasSuffix expects value and suffix")
	}
	s, ok1 := asString(args[0])
	p, ok2 := asString(args[1])
	return ok1 && ok2 && strings.HasSuffix(s, p), nil
}

func fnSplit(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("split expects value and separator")
	}
	s, ok1 := asString(args[0])
	sep, ok2 := asString(args[1])
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("split expects strings")
	}
	parts := strings.Split(s, sep)
	out := make([]any, len(parts))
	for i, part := range parts {
		out[i] = part
	}
	return out, nil
}

func fnJoin(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("join expects collection and separator")
	}
	sep, ok := asString(args[1])
	if !ok {
		return nil, fmt.Errorf("join separator must be string")
	}
	parts := make([]string, 0, collectionLen(args[0]))
	err := eachCollection(args[0], func(item any) error {
		parts = append(parts, fmt.Sprint(item))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return strings.Join(parts, sep), nil
}

func fnAbs(_ EvalContext, args ...any) (any, error) {
	v, err := oneFloat("abs", args)
	return math.Abs(v), err
}

func fnRound(_ EvalContext, args ...any) (any, error) {
	v, err := oneFloat("round", args)
	return math.Round(v), err
}

func fnFloor(_ EvalContext, args ...any) (any, error) {
	v, err := oneFloat("floor", args)
	return math.Floor(v), err
}

func fnCeil(_ EvalContext, args ...any) (any, error) {
	v, err := oneFloat("ceil", args)
	return math.Ceil(v), err
}

func fnClamp(_ EvalContext, args ...any) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("clamp expects value, min, max")
	}
	v, ok1 := asFloat(args[0])
	min, ok2 := asFloat(args[1])
	max, ok3 := asFloat(args[2])
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("clamp expects numeric arguments")
	}
	return math.Max(min, math.Min(max, v)), nil
}

func fnBetween(_ EvalContext, args ...any) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("between expects value, min, max")
	}
	lo, ok := compare(args[0], args[1])
	if !ok {
		return false, nil
	}
	hi, ok := compare(args[0], args[2])
	return ok && lo >= 0 && hi <= 0, nil
}

func fnNow(_ EvalContext, args ...any) (any, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("now expects no arguments")
	}
	return time.Now().UTC(), nil
}

func fnDate(_ EvalContext, args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("date expects 1 argument")
	}
	return parseTimeValue(args[0])
}

func fnBefore(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("before expects two date/time values")
	}
	a, err := parseTimeValue(args[0])
	if err != nil {
		return nil, err
	}
	b, err := parseTimeValue(args[1])
	if err != nil {
		return nil, err
	}
	return a.Before(b), nil
}

func fnAfter(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("after expects two date/time values")
	}
	a, err := parseTimeValue(args[0])
	if err != nil {
		return nil, err
	}
	b, err := parseTimeValue(args[1])
	if err != nil {
		return nil, err
	}
	return a.After(b), nil
}

func fnBetweenTime(_ EvalContext, args ...any) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("betweenTime expects value, start, end")
	}
	v, err := parseTimeValue(args[0])
	if err != nil {
		return nil, err
	}
	start, err := parseTimeValue(args[1])
	if err != nil {
		return nil, err
	}
	end, err := parseTimeValue(args[2])
	if err != nil {
		return nil, err
	}
	return (v.Equal(start) || v.After(start)) && (v.Equal(end) || v.Before(end)), nil
}

func fnAge(_ EvalContext, args ...any) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("age expects date/time and optional unit")
	}
	t, err := parseTimeValue(args[0])
	if err != nil {
		return nil, err
	}
	unit := "seconds"
	if len(args) == 2 {
		var ok bool
		unit, ok = asString(args[1])
		if !ok {
			return nil, fmt.Errorf("age unit must be string")
		}
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

func fnNullableMatch(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("nullableMatch expects route value and input value")
	}
	return args[0] == nil || equal(args[0], args[1]), nil
}

func fnRangeMatch(_ EvalContext, args ...any) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("rangeMatch expects min, max, value")
	}
	if args[0] != nil {
		c, ok := compare(args[0], args[2])
		if !ok || c > 0 {
			return false, nil
		}
	}
	if args[1] != nil {
		c, ok := compare(args[1], args[2])
		if !ok || c < 0 {
			return false, nil
		}
	}
	return true, nil
}

func fnActive(_ EvalContext, args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("active expects status")
	}
	s, ok := asString(args[0])
	return ok && strings.EqualFold(s, "active"), nil
}

func fnValidNow(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("validNow expects valid_from and valid_to")
	}
	now := time.Now().UTC()
	if args[0] != nil {
		from, err := parseTimeValue(args[0])
		if err != nil {
			return nil, err
		}
		if now.Before(from) {
			return false, nil
		}
	}
	if args[1] != nil {
		to, err := parseTimeValue(args[1])
		if err != nil {
			return nil, err
		}
		if now.After(to) {
			return false, nil
		}
	}
	return true, nil
}

func fnSpecificity(_ EvalContext, args ...any) (any, error) {
	score := 0.0
	for _, arg := range args {
		if arg != nil {
			score++
		}
	}
	return score, nil
}

func fnStableBucket(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("stableBucket expects key and modulo")
	}
	mod, ok := asFloat(args[1])
	if !ok || mod <= 0 {
		return nil, fmt.Errorf("stableBucket modulo must be positive")
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprint(args[0])))
	return float64(h.Sum64() % uint64(mod)), nil
}

func fnGet(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("get expects value and path")
	}
	path, ok := asString(args[1])
	if !ok {
		return nil, fmt.Errorf("get path must be string")
	}
	v, ok := lookupPath(args[0], path)
	if !ok {
		return nil, nil
	}
	return v, nil
}

func oneString(name string, args []any) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("%s expects 1 argument", name)
	}
	s, ok := asString(args[0])
	if !ok {
		return "", fmt.Errorf("%s expects string", name)
	}
	return s, nil
}

func oneFloat(name string, args []any) (float64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("%s expects 1 argument", name)
	}
	v, ok := asFloat(args[0])
	if !ok {
		return 0, fmt.Errorf("%s expects numeric value", name)
	}
	return v, nil
}

func parseTimeValue(v any) (time.Time, error) {
	switch x := v.(type) {
	case time.Time:
		return x, nil
	case int64:
		return time.Unix(x, 0).UTC(), nil
	case int:
		return time.Unix(int64(x), 0).UTC(), nil
	case float64:
		return time.Unix(int64(x), 0).UTC(), nil
	case string:
		if raw, ok := parsedTimeCache.Load(x); ok {
			return raw.(time.Time), nil
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"} {
			if t, err := time.Parse(layout, x); err == nil {
				t = t.UTC()
				parsedTimeCache.Store(x, t)
				return t, nil
			}
		}
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			t := time.Unix(n, 0).UTC()
			parsedTimeCache.Store(x, t)
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time from %v (%T)", v, v)
}
