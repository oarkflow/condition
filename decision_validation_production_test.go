package condition

import (
	"context"
	"testing"
)

func TestValidateDecisionPackageProductionDiagnostics(t *testing.T) {
	pkg := DecisionPackage{
		Name:    "bad",
		Version: "1",
		Datasets: []Dataset{{Name: "providers", Records: []DatasetRecord{
			{ID: "a"}, {ID: "b"},
		}}},
		Rankings: []Ranking{
			{Name: "route", Dataset: "missing", Selection: SelectionMode("sometimes")},
			{Name: "weighted", Dataset: "providers", Selection: SelectionWeighted, RuleSet: RuleSet{Name: "weighted", Rules: []Rule{{ID: "active", Condition: "true == true"}}}},
		},
		Optimizations: []Optimization{{Name: "opt", Ranking: "missing-ranking", Selection: SelectionMode("later")}},
		Actions:       []ActionDefinition{{Type: "notify"}, {Type: "notify"}},
		Tests:         []DecisionTestCase{{Name: "same", Expect: MapFacts{"effect": "allow"}}, {Name: "same", Expect: MapFacts{"effect": "allow"}}},
	}
	diags := ValidateDecisionPackage(pkg)
	assertDiag(t, diags, "rankings[0].dataset")
	assertDiag(t, diags, "rankings[0].selection")
	assertDiag(t, diags, "rankings[1].weight_path")
	assertDiag(t, diags, "optimizations[0].selection")
	assertDiag(t, diags, "optimizations[0].ranking")
	assertDiag(t, diags, "actions[1].type")
	assertDiag(t, diags, "tests[1].name")
}

func TestValidateProductionDecisionPackageRunsTests(t *testing.T) {
	pkg := DecisionPackage{
		Name:    "prod-pkg",
		Version: "1",
		Policies: []Policy{{Name: "review", DefaultEffect: EffectAllow, Rules: []PolicyRule{
			{ID: "deny", Effect: EffectDeny, Condition: `customer.blocked == true`},
		}}},
		Tests: []DecisionTestCase{{
			Name:     "blocked denied",
			Decision: "review",
			Context:  MapFacts{"customer": map[string]any{"blocked": true}},
			Expect:   MapFacts{"effect": "deny"},
		}},
	}
	result, err := ValidateProductionDecisionPackage(context.Background(), pkg)
	if err != nil {
		t.Fatalf("ValidateProductionDecisionPackage returned error: %v", err)
	}
	if result.Digest == "" || result.Tests == nil || !result.Tests.Passed {
		t.Fatalf("unexpected production validation result: %#v", result)
	}
}

func assertDiag(t *testing.T, diags []Diagnostic, path string) {
	t.Helper()
	for _, diag := range diags {
		if diag.Path == path {
			return
		}
	}
	t.Fatalf("missing diagnostic path %s in %#v", path, diags)
}
