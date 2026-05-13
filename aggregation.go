package condition

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

func registerAggregateFunctions(r *FunctionRegistry) {
	r.Register("count", aggregateCount)
	r.Register("countWhere", aggregateCountWhere)
	r.Register("sum", aggregateSum)
	r.Register("avg", aggregateAvg)
	r.Register("min", aggregateMin)
	r.Register("max", aggregateMax)
	r.Register("groupBy", aggregateGroupBy)
	r.Register("groupCount", aggregateGroupCount)
	r.Register("groupSum", aggregateGroupSum)
	r.Register("groupAvg", aggregateGroupAvg)
	r.Register("groupMin", aggregateGroupMin)
	r.Register("groupMax", aggregateGroupMax)
	r.Register("groupCountWhere", aggregateGroupCountWhere)
	r.Register("groupSumWhere", aggregateGroupSumWhere)
	r.Register("groupAvgWhere", aggregateGroupAvgWhere)
	r.Register("distinctCount", aggregateDistinctCount)
	r.Register("filter", aggregateFilter)
	r.Register("pluck", aggregatePluck)
	r.Register("first", aggregateFirst)
	r.Register("last", aggregateLast)
	r.Register("top", aggregateTop)
	r.Register("bottom", aggregateBottom)
	r.Register("percentile", aggregatePercentile)
	r.Register("sortBy", aggregateSortBy)
	r.Register("take", aggregateTake)
	r.Register("skip", aggregateSkip)
	r.Register("slice", aggregateSlice)
	r.Register("reverse", aggregateReverse)
	r.Register("distinct", aggregateDistinct)
	r.Register("any", aggregateAny)
	r.Register("all", aggregateAll)
	r.Register("none", aggregateNone)
}

func aggregateCount(_ EvalContext, args ...any) (any, error) {
	if len(args) == 1 {
		return float64(collectionLen(args[0])), nil
	}
	return aggregateCountWhere(EvalContext{}, args...)
}

func aggregateCountWhere(_ EvalContext, args ...any) (any, error) {
	if len(args) < 2 || len(args) > 4 {
		return nil, fmt.Errorf("countWhere expects collection, field, optional operator, optional value")
	}
	count := 0
	err := eachCollection(args[0], func(item any) error {
		ok, err := matchAggregateItem(item, args[1:])
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

func aggregateSum(_ EvalContext, args ...any) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("sum expects collection and optional field")
	}
	sum := 0.0
	err := eachAggregateNumber(args, func(v float64) {
		sum += v
	})
	return sum, err
}

func aggregateAvg(_ EvalContext, args ...any) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("avg expects collection and optional field")
	}
	sum := 0.0
	count := 0.0
	err := eachAggregateNumber(args, func(v float64) {
		sum += v
		count++
	})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return float64(0), nil
	}
	return sum / count, nil
}

func aggregateMin(_ EvalContext, args ...any) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("min expects collection and optional field")
	}
	min := 0.0
	seen := false
	err := eachAggregateNumber(args, func(v float64) {
		if !seen || v < min {
			min = v
			seen = true
		}
	})
	if err != nil {
		return nil, err
	}
	if !seen {
		return nil, nil
	}
	return min, nil
}

func aggregateMax(_ EvalContext, args ...any) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("max expects collection and optional field")
	}
	max := 0.0
	seen := false
	err := eachAggregateNumber(args, func(v float64) {
		if !seen || v > max {
			max = v
			seen = true
		}
	})
	if err != nil {
		return nil, err
	}
	if !seen {
		return nil, nil
	}
	return max, nil
}

func aggregateGroupBy(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("groupBy expects collection and group field")
	}
	groupField, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("groupBy group field must be a string")
	}
	out := map[string]any{}
	err := eachCollection(args[0], func(item any) error {
		key, ok := aggregateGroupKey(item, groupField)
		if !ok {
			return nil
		}
		current, _ := out[key].(float64)
		out[key] = current + 1
		return nil
	})
	return out, err
}

func aggregateGroupCount(_ EvalContext, args ...any) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("groupCount expects collection, group field, group value")
	}
	count := 0
	err := eachGroupItem(args[0], args[1], args[2], func(any) error {
		count++
		return nil
	})
	return float64(count), err
}

func aggregateGroupSum(_ EvalContext, args ...any) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("groupSum expects collection, group field, group value, numeric field")
	}
	sum := 0.0
	err := eachGroupNumber(args, func(v float64) {
		sum += v
	})
	return sum, err
}

func aggregateGroupAvg(_ EvalContext, args ...any) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("groupAvg expects collection, group field, group value, numeric field")
	}
	sum := 0.0
	count := 0.0
	err := eachGroupNumber(args, func(v float64) {
		sum += v
		count++
	})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return float64(0), nil
	}
	return sum / count, nil
}

func aggregateGroupMin(_ EvalContext, args ...any) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("groupMin expects collection, group field, group value, numeric field")
	}
	min := 0.0
	seen := false
	err := eachGroupNumber(args, func(v float64) {
		if !seen || v < min {
			min = v
			seen = true
		}
	})
	if err != nil {
		return nil, err
	}
	if !seen {
		return nil, nil
	}
	return min, nil
}

func aggregateGroupMax(_ EvalContext, args ...any) (any, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("groupMax expects collection, group field, group value, numeric field")
	}
	max := 0.0
	seen := false
	err := eachGroupNumber(args, func(v float64) {
		if !seen || v > max {
			max = v
			seen = true
		}
	})
	if err != nil {
		return nil, err
	}
	if !seen {
		return nil, nil
	}
	return max, nil
}

func aggregateDistinctCount(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("distinctCount expects collection and field")
	}
	field, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("distinctCount field must be a string")
	}
	seen := map[string]struct{}{}
	err := eachCollection(args[0], func(item any) error {
		key, ok := aggregateGroupKey(item, field)
		if ok {
			seen[key] = struct{}{}
		}
		return nil
	})
	return float64(len(seen)), err
}

func aggregateFilter(_ EvalContext, args ...any) (any, error) {
	if len(args) < 2 || len(args) > 4 {
		return nil, fmt.Errorf("filter expects collection, field, optional operator, optional value")
	}
	out := make([]any, 0)
	err := eachCollection(args[0], func(item any) error {
		ok, err := matchAggregateItem(item, args[1:])
		if err != nil {
			return err
		}
		if ok {
			out = append(out, item)
		}
		return nil
	})
	return out, err
}

func aggregatePluck(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("pluck expects collection and field")
	}
	field, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("pluck field must be a string")
	}
	out := make([]any, 0, collectionLen(args[0]))
	err := eachCollection(args[0], func(item any) error {
		v, ok := lookupPath(item, field)
		if ok {
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

func aggregateFirst(_ EvalContext, args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("first expects collection")
	}
	var out any
	found := false
	err := eachCollection(args[0], func(item any) error {
		if !found {
			out = item
			found = true
		}
		return nil
	})
	return out, err
}

func aggregateLast(_ EvalContext, args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("last expects collection")
	}
	var out any
	err := eachCollection(args[0], func(item any) error {
		out = item
		return nil
	})
	return out, err
}

func aggregateTop(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("top expects collection and numeric field")
	}
	return topBottom(args[0], args[1], true)
}

func aggregateBottom(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("bottom expects collection and numeric field")
	}
	return topBottom(args[0], args[1], false)
}

func aggregatePercentile(_ EvalContext, args ...any) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("percentile expects collection, numeric field, p")
	}
	values, err := collectNumbers(args[0], args[1])
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	p, ok := asFloat(args[2])
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
	sortFloat64s(values)
	idx := int(math.Round(p * float64(len(values)-1)))
	return values[idx], nil
}

func aggregateSortBy(_ EvalContext, args ...any) (any, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("sortBy expects collection, field, optional direction")
	}
	field, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("sortBy field must be string")
	}
	desc := false
	if len(args) == 3 {
		direction, ok := asString(args[2])
		if !ok {
			return nil, fmt.Errorf("sortBy direction must be string")
		}
		switch strings.ToLower(direction) {
		case "", "asc", "ascending":
		case "desc", "descending":
			desc = true
		default:
			return nil, fmt.Errorf("sortBy direction must be asc or desc")
		}
	}
	items, err := collectCollection(args[0])
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, leftOK := lookupPath(items[i], field)
		right, rightOK := lookupPath(items[j], field)
		if !leftOK || left == nil {
			return false
		}
		if !rightOK || right == nil {
			return true
		}
		cmp := compareSortValues(left, right)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	return items, nil
}

func aggregateTake(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("take expects collection and count")
	}
	items, err := collectCollection(args[0])
	if err != nil {
		return nil, err
	}
	n, err := collectionIndex("take", args[1], len(items))
	if err != nil {
		return nil, err
	}
	if n > len(items) {
		n = len(items)
	}
	out := make([]any, n)
	copy(out, items[:n])
	return out, nil
}

func aggregateSkip(_ EvalContext, args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("skip expects collection and count")
	}
	items, err := collectCollection(args[0])
	if err != nil {
		return nil, err
	}
	n, err := collectionIndex("skip", args[1], len(items))
	if err != nil {
		return nil, err
	}
	if n > len(items) {
		n = len(items)
	}
	out := make([]any, len(items)-n)
	copy(out, items[n:])
	return out, nil
}

func aggregateSlice(_ EvalContext, args ...any) (any, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("slice expects collection, start, optional end")
	}
	items, err := collectCollection(args[0])
	if err != nil {
		return nil, err
	}
	start, err := collectionIndex("slice", args[1], len(items))
	if err != nil {
		return nil, err
	}
	end := len(items)
	if len(args) == 3 {
		end, err = collectionIndex("slice", args[2], len(items))
		if err != nil {
			return nil, err
		}
	}
	if start > len(items) {
		start = len(items)
	}
	if end > len(items) {
		end = len(items)
	}
	if end < start {
		end = start
	}
	out := make([]any, end-start)
	copy(out, items[start:end])
	return out, nil
}

func aggregateReverse(_ EvalContext, args ...any) (any, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("reverse expects collection")
	}
	items, err := collectCollection(args[0])
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

func aggregateDistinct(_ EvalContext, args ...any) (any, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("distinct expects collection and optional field")
	}
	field := ""
	if len(args) == 2 {
		var ok bool
		field, ok = args[1].(string)
		if !ok {
			return nil, fmt.Errorf("distinct field must be a string")
		}
	}
	seen := map[string]struct{}{}
	out := make([]any, 0, collectionLen(args[0]))
	err := eachCollection(args[0], func(item any) error {
		value := item
		if field != "" {
			var ok bool
			value, ok = lookupPath(item, field)
			if !ok {
				return nil
			}
		}
		key := distinctKey(value)
		if _, ok := seen[key]; ok {
			return nil
		}
		seen[key] = struct{}{}
		out = append(out, value)
		return nil
	})
	return out, err
}

func aggregateAny(_ EvalContext, args ...any) (any, error) {
	if len(args) < 2 || len(args) > 4 {
		return nil, fmt.Errorf("any expects collection, field, optional operator, optional value")
	}
	matched := false
	err := eachCollection(args[0], func(item any) error {
		if matched {
			return nil
		}
		ok, err := matchAggregateItem(item, args[1:])
		if err != nil {
			return err
		}
		matched = ok
		return nil
	})
	return matched, err
}

func aggregateAll(_ EvalContext, args ...any) (any, error) {
	if len(args) < 2 || len(args) > 4 {
		return nil, fmt.Errorf("all expects collection, field, optional operator, optional value")
	}
	seen := false
	matched := true
	err := eachCollection(args[0], func(item any) error {
		if !matched {
			return nil
		}
		seen = true
		ok, err := matchAggregateItem(item, args[1:])
		if err != nil {
			return err
		}
		matched = ok
		return nil
	})
	return seen && matched, err
}

func aggregateNone(ctx EvalContext, args ...any) (any, error) {
	v, err := aggregateAny(ctx, args...)
	if err != nil {
		return nil, err
	}
	return !truthy(v), nil
}

func aggregateGroupCountWhere(_ EvalContext, args ...any) (any, error) {
	if len(args) != 6 {
		return nil, fmt.Errorf("groupCountWhere expects collection, group field, group value, field, op, value")
	}
	count := 0
	err := eachGroupWhereItem(args, func(any) error {
		count++
		return nil
	})
	return float64(count), err
}

func aggregateGroupSumWhere(_ EvalContext, args ...any) (any, error) {
	if len(args) != 7 {
		return nil, fmt.Errorf("groupSumWhere expects collection, group field, group value, field, op, value, numeric field")
	}
	sum := 0.0
	err := eachGroupWhereItem(args[:6], func(item any) error {
		field, ok := args[6].(string)
		if !ok {
			return fmt.Errorf("groupSumWhere numeric field must be a string")
		}
		v, ok := lookupPath(item, field)
		if !ok || v == nil {
			return nil
		}
		n, ok := asFloat(v)
		if !ok {
			return fmt.Errorf("groupSumWhere value %v (%T) is not numeric", v, v)
		}
		sum += n
		return nil
	})
	return sum, err
}

func aggregateGroupAvgWhere(_ EvalContext, args ...any) (any, error) {
	if len(args) != 7 {
		return nil, fmt.Errorf("groupAvgWhere expects collection, group field, group value, field, op, value, numeric field")
	}
	sum := 0.0
	count := 0.0
	err := eachGroupWhereItem(args[:6], func(item any) error {
		field, ok := args[6].(string)
		if !ok {
			return fmt.Errorf("groupAvgWhere numeric field must be a string")
		}
		v, ok := lookupPath(item, field)
		if !ok || v == nil {
			return nil
		}
		n, ok := asFloat(v)
		if !ok {
			return fmt.Errorf("groupAvgWhere value %v (%T) is not numeric", v, v)
		}
		sum += n
		count++
		return nil
	})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return float64(0), nil
	}
	return sum / count, nil
}

func eachGroupNumber(args []any, fn func(float64)) error {
	valueField, ok := args[3].(string)
	if !ok {
		return fmt.Errorf("group numeric field must be a string")
	}
	return eachGroupItem(args[0], args[1], args[2], func(item any) error {
		v, ok := lookupPath(item, valueField)
		if !ok || v == nil {
			return nil
		}
		n, ok := asFloat(v)
		if !ok {
			return fmt.Errorf("group aggregation value %v (%T) is not numeric", v, v)
		}
		fn(n)
		return nil
	})
}

func eachGroupWhereItem(args []any, fn func(any) error) error {
	return eachGroupItem(args[0], args[1], args[2], func(item any) error {
		ok, err := matchAggregateItem(item, args[3:])
		if err != nil || !ok {
			return err
		}
		return fn(item)
	})
}

func eachGroupItem(collection, field, value any, fn func(any) error) error {
	groupField, ok := field.(string)
	if !ok {
		return fmt.Errorf("group field must be a string")
	}
	return eachCollection(collection, func(item any) error {
		key, ok := lookupPath(item, groupField)
		if !ok {
			return nil
		}
		if equal(key, value) {
			return fn(item)
		}
		return nil
	})
}

func topBottom(collection, fieldArg any, top bool) (any, error) {
	field, ok := fieldArg.(string)
	if !ok {
		return nil, fmt.Errorf("top/bottom field must be string")
	}
	var out any
	best := 0.0
	seen := false
	err := eachCollection(collection, func(item any) error {
		v, ok := lookupPath(item, field)
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

func collectNumbers(collection, fieldArg any) ([]float64, error) {
	field, ok := fieldArg.(string)
	if !ok {
		return nil, fmt.Errorf("numeric field must be string")
	}
	values := make([]float64, 0, collectionLen(collection))
	err := eachCollection(collection, func(item any) error {
		v, ok := lookupPath(item, field)
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
	return values, err
}

func sortFloat64s(values []float64) {
	for i := 1; i < len(values); i++ {
		v := values[i]
		j := i - 1
		for j >= 0 && values[j] > v {
			values[j+1] = values[j]
			j--
		}
		values[j+1] = v
	}
}

func collectCollection(v any) ([]any, error) {
	items := make([]any, 0, collectionLen(v))
	err := eachCollection(v, func(item any) error {
		items = append(items, item)
		return nil
	})
	return items, err
}

func collectionIndex(name string, v any, length int) (int, error) {
	n, ok := asFloat(v)
	if !ok {
		return 0, fmt.Errorf("%s index/count must be numeric", name)
	}
	i := int(n)
	if i < 0 {
		i = length + i
	}
	if i < 0 {
		return 0, nil
	}
	return i, nil
}

func compareSortValues(a, b any) int {
	if c, ok := compare(a, b); ok {
		return c
	}
	at, aerr := parseTimeValue(a)
	bt, berr := parseTimeValue(b)
	if aerr == nil && berr == nil {
		switch {
		case at.Before(bt):
			return -1
		case at.After(bt):
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
}

func distinctKey(v any) string {
	return fmt.Sprintf("%T:%#v", v, v)
}

func aggregateGroupKey(item any, field string) (string, bool) {
	key, ok := lookupPath(item, field)
	if !ok || key == nil {
		return "", false
	}
	return fmt.Sprint(key), true
}

func eachAggregateNumber(args []any, fn func(float64)) error {
	field := ""
	if len(args) == 2 {
		var ok bool
		field, ok = args[1].(string)
		if !ok {
			return fmt.Errorf("aggregation field must be a string")
		}
	}
	return eachCollection(args[0], func(item any) error {
		v := item
		if field != "" {
			var ok bool
			v, ok = lookupPath(item, field)
			if !ok || v == nil {
				return nil
			}
		}
		n, ok := asFloat(v)
		if !ok {
			return fmt.Errorf("aggregation value %v (%T) is not numeric", v, v)
		}
		fn(n)
		return nil
	})
}

func matchAggregateItem(item any, args []any) (bool, error) {
	field, ok := args[0].(string)
	if !ok {
		return false, fmt.Errorf("aggregate predicate field must be a string")
	}
	v, ok := lookupPath(item, field)
	if !ok {
		v = nil
	}
	switch len(args) {
	case 1:
		return truthy(v), nil
	case 2:
		return equal(v, args[1]), nil
	case 3:
		op, ok := args[1].(string)
		if !ok {
			return false, fmt.Errorf("aggregate predicate operator must be a string")
		}
		return evalOperator(op, v, args[2])
	default:
		return false, fmt.Errorf("invalid aggregate predicate")
	}
}

func collectionLen(v any) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case []any:
		return len(x)
	case []string:
		return len(x)
	case []int:
		return len(x)
	case []float64:
		return len(x)
	case map[string]any:
		return len(x)
	case MapFacts:
		return len(x)
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return 0
	}
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len()
	default:
		return 0
	}
}

func eachCollection(v any, fn func(any) error) error {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if err := fn(item); err != nil {
				return err
			}
		}
		return nil
	case []string:
		for _, item := range x {
			if err := fn(item); err != nil {
				return err
			}
		}
		return nil
	case []int:
		for _, item := range x {
			if err := fn(item); err != nil {
				return err
			}
		}
		return nil
	case []float64:
		for _, item := range x {
			if err := fn(item); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for _, item := range x {
			if err := fn(item); err != nil {
				return err
			}
		}
		return nil
	case MapFacts:
		for _, item := range x {
			if err := fn(item); err != nil {
				return err
			}
		}
		return nil
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil
	}
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if err := fn(rv.Index(i).Interface()); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		iter := rv.MapRange()
		for iter.Next() {
			if err := fn(iter.Value().Interface()); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("expected collection, got %T", v)
	}
}
