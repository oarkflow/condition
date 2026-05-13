package condition

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
)

type MapFacts map[string]any

func (m MapFacts) Get(path string) (any, bool) { return lookupPath(map[string]any(m), path) }
func (m MapFacts) GetPath(parts []string) (any, bool) {
	return lookupParts(map[string]any(m), parts)
}

type FactFunc func(path string) (any, bool)

func (f FactFunc) Get(path string) (any, bool) { return f(path) }

type StructFacts struct{ value any }

func NewStructFacts(v any) StructFacts { return StructFacts{value: v} }

func (s StructFacts) Get(path string) (any, bool) { return lookupPath(s.value, path) }
func (s StructFacts) GetPath(parts []string) (any, bool) {
	return lookupParts(s.value, parts)
}

type ChainedFacts []Facts

func Chain(facts ...Facts) ChainedFacts { return ChainedFacts(facts) }

func (c ChainedFacts) Get(path string) (any, bool) {
	for _, f := range c {
		if f == nil {
			continue
		}
		if v, ok := f.Get(path); ok {
			return v, true
		}
	}
	return nil, false
}

func (c ChainedFacts) GetPath(parts []string) (any, bool) {
	for _, f := range c {
		if f == nil {
			continue
		}
		if pf, ok := f.(PathFacts); ok {
			if v, ok := pf.GetPath(parts); ok {
				return v, true
			}
			continue
		}
		if v, ok := f.Get(strings.Join(parts, ".")); ok {
			return v, true
		}
	}
	return nil, false
}

var fieldCache sync.Map

func lookupPath(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}
	return lookupParts(v, strings.Split(path, "."))
}

func lookupParts(v any, parts []string) (any, bool) {
	cur := v
	for _, part := range parts {
		if cur == nil {
			return nil, false
		}
		if m, ok := cur.(MapFacts); ok {
			n, ok := m[part]
			if !ok {
				return nil, false
			}
			cur = n
			continue
		}
		if m, ok := cur.(map[string]any); ok {
			n, ok := m[part]
			if !ok {
				return nil, false
			}
			cur = n
			continue
		}
		rv := reflect.ValueOf(cur)
		for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
			if rv.IsNil() {
				return nil, false
			}
			rv = rv.Elem()
		}
		switch rv.Kind() {
		case reflect.Map:
			if rv.Type().Key().Kind() != reflect.String {
				return nil, false
			}
			mv := rv.MapIndex(reflect.ValueOf(part))
			if !mv.IsValid() {
				return nil, false
			}
			cur = mv.Interface()
		case reflect.Struct:
			idx, ok := cachedFieldIndex(rv.Type(), part)
			if !ok {
				return nil, false
			}
			fv := rv.FieldByIndex(idx)
			if !fv.CanInterface() {
				return nil, false
			}
			cur = fv.Interface()
		case reflect.Slice, reflect.Array:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= rv.Len() {
				return nil, false
			}
			cur = rv.Index(i).Interface()
		default:
			return nil, false
		}
	}
	return cur, true
}

func cachedFieldIndex(t reflect.Type, name string) ([]int, bool) {
	raw, _ := fieldCache.LoadOrStore(t, buildFieldMap(t))
	m := raw.(map[string][]int)
	idx, ok := m[name]
	if ok {
		return idx, true
	}
	idx, ok = m[strings.ToLower(name)]
	return idx, ok
}

func buildFieldMap(t reflect.Type) map[string][]int {
	m := make(map[string][]int)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		name := f.Name
		if tag := f.Tag.Get("json"); tag != "" {
			if p := strings.Split(tag, ",")[0]; p != "" && p != "-" {
				name = p
			}
		}
		m[name] = f.Index
		m[strings.ToLower(name)] = f.Index
		m[f.Name] = f.Index
		m[strings.ToLower(f.Name)] = f.Index
	}
	return m
}
