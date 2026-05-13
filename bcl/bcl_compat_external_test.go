package bcl

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/oarkflow/condition"
	"github.com/oarkflow/condition/yamlx"
)

func TestDecisionPackageFormatCompatibilityDigests(t *testing.T) {
	goPkg := condition.DecisionPackage{
		Name:        "compat",
		Version:     "1",
		Environment: "test",
		Schemas: map[string]condition.Schema{"review": {
			Required: []string{"customer.blacklisted"},
			Types:    map[string]string{"customer.blacklisted": "bool"},
		}},
		Policies: []condition.Policy{{Name: "review", DefaultEffect: condition.EffectAllow, Rules: []condition.PolicyRule{{
			ID:        "deny-blacklisted",
			Effect:    condition.EffectDeny,
			Condition: "customer.blacklisted == true",
			Reason:    "customer is blacklisted",
		}}}},
		RuleSets: []condition.RuleSet{{Name: "review", Rules: []condition.Rule{{
			ID:        "1001",
			Condition: "transaction.amount > 100000",
			Decision:  "require_review",
			Score:     40,
		}}}},
	}
	bcl := `
module "compat" {
  version = "1"
  environment = "test"
  schema "review" {
    required customer.blacklisted
    type customer.blacklisted bool
  }
  policy "review" {
    default allow
    deny "deny-blacklisted" when { customer.blacklisted == true } reason "customer is blacklisted"
  }
  rule_set "review" {
    rule "1001" {
      when { transaction.amount > 100000 }
      then { decision = "require_review" score += 40 }
    }
  }
}
`
	jsonData, err := json.Marshal(goPkg)
	if err != nil {
		t.Fatal(err)
	}
	yamlData := []byte(`
name: compat
version: "1"
environment: test
schemas:
  review:
    required: [customer.blacklisted]
    types:
      customer.blacklisted: bool
policies:
  - name: review
    default_effect: allow
    rules:
      - id: deny-blacklisted
        effect: deny
        condition: customer.blacklisted == true
        reason: customer is blacklisted
rule_sets:
  - name: review
    rules:
      - id: "1001"
        condition: transaction.amount > 100000
        decision: require_review
        score: 40
`)
	fromBCL, err := ParsePackage([]byte(bcl))
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := condition.LoadDecisionPackage(context.Background(), condition.BytesSource(jsonData), condition.JSONDecoder[condition.DecisionPackage]())
	if err != nil {
		t.Fatal(err)
	}
	fromYAML, err := condition.LoadDecisionPackage(context.Background(), condition.BytesSource(yamlData), yamlx.JSONTagDecoder[condition.DecisionPackage]())
	if err != nil {
		t.Fatal(err)
	}
	want, _ := condition.PackageDigest(goPkg)
	for name, pkg := range map[string]condition.DecisionPackage{"bcl": fromBCL, "json": fromJSON, "yaml": fromYAML} {
		got, _ := condition.PackageDigest(pkg)
		if got != want {
			t.Fatalf("%s digest = %s, want %s\npkg=%#v", name, got, want, pkg)
		}
	}
}
