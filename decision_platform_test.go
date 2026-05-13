package condition

import (
	"context"
	"strings"
	"testing"
)

func TestDecisionPackageDigestStableAndChanges(t *testing.T) {
	pkg := testDecisionPackage()
	a, err := PackageDigest(pkg)
	if err != nil {
		t.Fatalf("PackageDigest returned error: %v", err)
	}
	b, err := PackageDigest(pkg)
	if err != nil {
		t.Fatalf("PackageDigest repeat returned error: %v", err)
	}
	if a != b {
		t.Fatalf("digest changed across identical package: %s != %s", a, b)
	}
	pkg.RuleSets[0].Rules[0].Score = 99
	c, err := PackageDigest(pkg)
	if err != nil {
		t.Fatalf("PackageDigest changed returned error: %v", err)
	}
	if c == a {
		t.Fatal("digest did not change after executable rule change")
	}
}

func TestDecisionOrchestratorEndToEndPolicyRankingWorkflowExplanation(t *testing.T) {
	orchestrator := NewDecisionOrchestrator()
	if err := orchestrator.AddPackage(testDecisionPackage()); err != nil {
		t.Fatalf("AddPackage returned error: %v", err)
	}
	res, err := orchestrator.Evaluate(context.Background(), DecisionRequest{
		PackageName: "fraud-intelligence",
		Decision:    "fraud-review",
		Context: MapFacts{
			"customer":    map[string]any{"blacklisted": false, "country": "NP", "tier": "standard"},
			"transaction": map[string]any{"amount": 125000, "velocity_10m": 7},
			"device":      map[string]any{"is_new": true},
		},
		Candidates: []Candidate{
			{ID: "standard", Name: "Standard Queue", Facts: MapFacts{"active": true, "skills": []string{"payments"}, "priority": 5, "load": 0.4, "sla": 30}},
			{ID: "aml", Name: "AML Queue", Facts: MapFacts{"active": true, "skills": []string{"payments", "aml"}, "priority": 9, "load": 0.2, "sla": 10}},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if res.Effect != EffectRequireReview || res.Allowed {
		t.Fatalf("effect/allowed = %s/%v, want require_review/false", res.Effect, res.Allowed)
	}
	if res.Score <= 0 {
		t.Fatalf("score = %v, want positive risk score", res.Score)
	}
	if res.Rank == nil || res.Rank.ID != "aml" {
		t.Fatalf("rank = %#v, want aml winner", res.Rank)
	}
	if res.Digest == "" || res.Audit.PackageDigest != res.Digest || res.Audit.ResultFingerprint == "" {
		t.Fatalf("missing digest/audit: %#v", res.Audit)
	}
	if len(res.Explanation.PolicyEffects) == 0 || len(res.Explanation.MatchedRules) == 0 || len(res.Explanation.Evidence) == 0 {
		t.Fatalf("incomplete explanation: %#v", res.Explanation)
	}
	if len(res.Explanation.FactEvidence) == 0 {
		t.Fatalf("expected fact evidence: %#v", res.Explanation)
	}
}

func TestDecisionPackageRichSchemaValidation(t *testing.T) {
	minAmount := 10.0
	maxAmount := 1000.0
	minLen := 2
	pkg := DecisionPackage{
		Name: "schema-rich",
		Schemas: map[string]Schema{"eligibility": {
			Rules: map[string]SchemaRule{
				"transaction.amount": {Type: "number", Required: true, Min: &minAmount, Max: &maxAmount},
				"customer.country":   {Type: "string", Required: true, Enum: []any{"NP", "US"}, MinLength: &minLen},
				"customer.tags":      {Type: "array"},
			},
		}},
		Policies: []Policy{{Name: "eligibility", DefaultEffect: EffectAllow}},
	}
	orchestrator := NewDecisionOrchestrator()
	if err := orchestrator.AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage returned error: %v", err)
	}
	_, err := orchestrator.Evaluate(context.Background(), DecisionRequest{
		PackageName: "schema-rich",
		Decision:    "eligibility",
		Context: MapFacts{
			"transaction": map[string]any{"amount": 25},
			"customer":    map[string]any{"country": "NP", "tags": []any{"vip"}},
		},
	})
	if err != nil {
		t.Fatalf("valid schema context rejected: %v", err)
	}
	_, err = orchestrator.Evaluate(context.Background(), DecisionRequest{
		PackageName: "schema-rich",
		Decision:    "eligibility",
		Context: MapFacts{
			"transaction": map[string]any{"amount": 5000},
			"customer":    map[string]any{"country": "FR", "tags": []any{"vip"}},
		},
	})
	if err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestDecisionPolicyLatticeDenyOverridesAllow(t *testing.T) {
	pkg := testDecisionPackage()
	req := DecisionRequest{
		PackageName: "fraud-intelligence",
		Decision:    "fraud-review",
		Context: MapFacts{
			"customer":    map[string]any{"blacklisted": true, "country": "NP"},
			"transaction": map[string]any{"amount": 10, "velocity_10m": 0},
			"device":      map[string]any{"is_new": false},
		},
	}
	orchestrator := NewDecisionOrchestrator()
	if err := orchestrator.AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage returned error: %v", err)
	}
	res, err := orchestrator.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if res.Effect != EffectDeny || res.Allowed {
		t.Fatalf("effect/allowed = %s/%v, want deny/false", res.Effect, res.Allowed)
	}
}

func TestDecisionSimulationDiff(t *testing.T) {
	baseline := testDecisionPackage()
	candidate := testDecisionPackage()
	candidate.Version = "2"
	candidate.RuleSets[0].Rules[0].Condition = `transaction.amount > 200000`
	orchestrator := NewDecisionOrchestrator()
	sim, err := orchestrator.Simulate(context.Background(), SimulationRequest{
		Baseline:  &baseline,
		Candidate: &candidate,
		Cases: []DecisionRequest{{
			Decision: "fraud-review",
			Context: MapFacts{
				"customer":    map[string]any{"blacklisted": false, "country": "NP"},
				"transaction": map[string]any{"amount": 125000, "velocity_10m": 0},
				"device":      map[string]any{"is_new": false},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Simulate returned error: %v", err)
	}
	if sim.Summary.Total != 1 || sim.Summary.ScoreChanges != 1 {
		t.Fatalf("summary = %#v, want one score change", sim.Summary)
	}
	if !sim.Cases[0].Diff.ScoreChanged {
		t.Fatalf("diff = %#v, want score changed", sim.Cases[0].Diff)
	}
}

func TestDecisionPackageLoadingJSONAndCompare(t *testing.T) {
	raw := `{"name":"loaded","version":"1","rule_sets":[{"name":"eligibility","rules":[{"id":"1","condition":"user.age >= 18","decision":"allow"}]}]}`
	pkg, err := LoadDecisionPackage(context.Background(), StringSource(raw), JSONDecoder[DecisionPackage]())
	if err != nil {
		t.Fatalf("LoadDecisionPackage returned error: %v", err)
	}
	if pkg.Name != "loaded" || len(pkg.RuleSets) != 1 {
		t.Fatalf("loaded package = %#v", pkg)
	}
	other := testLoadedPackage()
	other.RuleSets[0].Rules[0].Decision = "deny"
	diff, err := NewDecisionOrchestrator().ComparePackages(pkg, other)
	if err != nil {
		t.Fatalf("ComparePackages returned error: %v", err)
	}
	if !diff.Changed || !strings.Contains(strings.Join(diff.ChangedParts, ","), "rule_sets") {
		t.Fatalf("diff = %#v, want changed rule_sets", diff)
	}
}

func testDecisionPackage() DecisionPackage {
	return DecisionPackage{
		Name:        "fraud-intelligence",
		Version:     "1",
		Environment: "test",
		Schemas: map[string]Schema{"fraud-review": {
			Required: []string{"customer.blacklisted", "transaction.amount"},
			Types:    map[string]string{"transaction.amount": "number"},
		}},
		Policies: []Policy{{
			Name:          "fraud-review",
			DefaultEffect: EffectAllow,
			Rules: []PolicyRule{
				{ID: "deny-blacklist", Effect: EffectDeny, Condition: `customer.blacklisted == true`, Reason: "customer is blacklisted"},
				{ID: "review-country", Effect: EffectRequireReview, Condition: `customer.country == "NP"`, Reason: "manual review market"},
			},
		}},
		RuleSets: []RuleSet{{
			Name: "fraud-review",
			Rules: []Rule{
				{ID: "1001", Condition: `transaction.amount > 100000`, Score: 40, Decision: "require_review", Reason: "high value transaction", Actions: []Action{{Type: "notify", Payload: map[string]any{"team": "compliance"}}}},
				{ID: "1002", Condition: `transaction.velocity_10m > 5`, Score: 30, Reason: "velocity threshold exceeded"},
				{ID: "1003", Condition: `device.is_new == true`, Score: 15, Reason: "new device"},
			},
		}},
		Rankings: []Ranking{{
			Name:            "fraud-review",
			Selection:       SelectionBest,
			PriorityPath:    "provider.priority",
			SpecificityPath: "provider.sla",
			CostPath:        "provider.load",
			RuleSet: RuleSet{
				Name: "queue-ranking",
				Rules: []Rule{
					{ID: "active", Condition: `provider.active == true`, Reason: "queue inactive"},
					{ID: "skill", Condition: `"payments" in provider.skills`, Reason: "missing payment skill"},
				},
				ScoreRules: []ScoreRule{
					{ID: "priority", Metric: "provider.priority", Weight: 0.7, Normalize: Normalize{Min: 1, Max: 10}},
					{ID: "load", Metric: "provider.load", Weight: 0.3, Direction: LowerBetter, Normalize: Normalize{Min: 0, Max: 1}},
				},
			},
		}},
		Workflows: []Workflow{{
			Name:       "fraud-review-workflow",
			StartStage: "intake",
			Stages: []Stage{{
				Name:  "intake",
				Rules: []Rule{{ID: "2001", Condition: `transaction.amount > 100000`, Decision: "escalate", NextStage: "", Reason: "escalate high value case"}},
			}},
		}},
		Optimizations: []Optimization{{
			Name:      "queue-optimization",
			Decision:  "fraud-review",
			Goal:      "maximize reviewer readiness while minimizing queue load",
			Ranking:   "fraud-review",
			Selection: SelectionBest,
		}},
		Actions: []ActionDefinition{
			{Type: "notify", Description: "Send an operational notification"},
			{Type: "escalate", Description: "Move the case to a senior review path"},
		},
		Tests: []DecisionTestCase{{
			Name:     "blacklisted customers are denied",
			Decision: "fraud-review",
			Context: MapFacts{
				"customer":    map[string]any{"blacklisted": true, "country": "NP"},
				"transaction": map[string]any{"amount": 1000},
				"device":      map[string]any{"is_new": false},
			},
			Expect: MapFacts{"effect": string(EffectDeny)},
		}},
	}
}

func testLoadedPackage() DecisionPackage {
	return DecisionPackage{
		Name:    "loaded",
		Version: "1",
		RuleSets: []RuleSet{{
			Name:  "eligibility",
			Rules: []Rule{{ID: "1", Condition: `user.age >= 18`, Decision: "allow"}},
		}},
	}
}
