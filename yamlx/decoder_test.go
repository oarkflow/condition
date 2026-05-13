package yamlx

import (
	"testing"

	"github.com/oarkflow/condition"
)

func TestJSONTagDecoderRuleSet(t *testing.T) {
	data := []byte(`
name: yaml
rules:
  - id: adult
    condition: user.age >= 18
    stop_on_match: true
`)
	loaded, err := JSONTagDecoder[condition.RuleSet]().Decode(data)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if loaded.Name != "yaml" || len(loaded.Rules) != 1 {
		t.Fatalf("unexpected ruleset: %#v", loaded)
	}
	if !loaded.Rules[0].StopOnMatch {
		t.Fatalf("stop_on_match was not decoded: %#v", loaded.Rules[0])
	}
}
