package yamlx

import (
	"encoding/json"

	"github.com/oarkflow/condition"
	"gopkg.in/yaml.v3"
)

func Decoder[T any]() condition.Decoder[T] {
	return condition.DecoderFunc[T](func(data []byte) (T, error) {
		var out T
		if err := yaml.Unmarshal(data, &out); err != nil {
			return out, err
		}
		return out, nil
	})
}

func JSONTagDecoder[T any]() condition.Decoder[T] {
	return condition.DecoderFunc[T](func(data []byte) (T, error) {
		var raw any
		var out T
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return out, err
		}
		normalized := normalizeYAML(raw)
		jsonData, err := json.Marshal(normalized)
		if err != nil {
			return out, err
		}
		err = json.Unmarshal(jsonData, &out)
		return out, err
	})
}

func normalizeYAML(v any) any {
	switch x := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			if key, ok := k.(string); ok {
				out[key] = normalizeYAML(v)
			}
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = normalizeYAML(v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = normalizeYAML(v)
		}
		return out
	default:
		return x
	}
}
