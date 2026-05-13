package condition

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func truthy(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return rv.Len() > 0
		}
		return true
	}
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	}
	return false
}

func equal(a, b any) bool {
	if af, ok := asFloat(a); ok {
		if bf, ok := asFloat(b); ok {
			return af == bf
		}
	}
	return reflect.DeepEqual(a, b)
}

func compare(a, b any) (int, bool) {
	if af, ok := asFloat(a); ok {
		if bf, ok := asFloat(b); ok {
			if af < bf {
				return -1, true
			}
			if af > bf {
				return 1, true
			}
			return 0, true
		}
	}
	as, aok := asString(a)
	bs, bok := asString(b)
	if aok && bok {
		return strings.Compare(as, bs), true
	}
	return 0, false
}

func contains(container, item any) (bool, error) {
	if s, ok := asString(container); ok {
		sub, ok := asString(item)
		if !ok {
			return false, fmt.Errorf("contains string expects string item")
		}
		return strings.Contains(s, sub), nil
	}
	switch c := container.(type) {
	case []string:
		it, ok := asString(item)
		if !ok {
			return false, nil
		}
		for _, v := range c {
			if v == it {
				return true, nil
			}
		}
		return false, nil
	case []int:
		it, ok := asFloat(item)
		if !ok {
			return false, nil
		}
		for _, v := range c {
			if float64(v) == it {
				return true, nil
			}
		}
		return false, nil
	case []float64:
		it, ok := asFloat(item)
		if !ok {
			return false, nil
		}
		for _, v := range c {
			if v == it {
				return true, nil
			}
		}
		return false, nil
	case []any:
		for _, v := range c {
			if equal(v, item) {
				return true, nil
			}
		}
		return false, nil
	case map[string]any:
		key, ok := asString(item)
		if !ok {
			return false, nil
		}
		_, ok = c[key]
		return ok, nil
	}
	rv := reflect.ValueOf(container)
	if !rv.IsValid() {
		return false, nil
	}
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return false, nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if equal(rv.Index(i).Interface(), item) {
				return true, nil
			}
		}
		return false, nil
	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			if key, ok := asString(item); ok {
				return rv.MapIndex(reflect.ValueOf(key)).IsValid(), nil
			}
		}
		return false, nil
	default:
		return false, fmt.Errorf("contains unsupported for %T", container)
	}
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func asString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case fmt.Stringer:
		return x.String(), true
	default:
		return "", false
	}
}
