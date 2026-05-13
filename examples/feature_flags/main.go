package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func main() {
	ctx := context.Background()
	facts := condition.MapFacts{
		"account": map[string]any{"id": "acct_42", "tier": "enterprise", "region": "US", "seat_count": 120, "status": "active"},
		"user":    map[string]any{"id": "user_99", "role": "admin", "email": "admin@example.com"},
		"feature": map[string]any{"key": "ai-workflows", "rollout_percent": 35, "min_seats": 25},
		"lists":   map[string]any{"allow_accounts": []string{"acct_42"}, "deny_accounts": []string{"acct_bad"}, "internal_domains": []string{"example.com"}},
	}
	vars := condition.MapFacts{"maxSeats": 500, "bucketModulo": 100}

	expr := condition.MustCompile(`
		account.status == "active" and
		account.seat_count <= $maxSeats and
		(
			account.id in lists.allow_accounts or
			(
				account.tier in ["enterprise", "business"] and
				account.region in ["US", "CA", "GB"] and
				stableBucket(account.id, $bucketModulo) < feature.rollout_percent
			)
		) and
		account.id not in lists.deny_accounts
	`)
	matched, err := expr.EvalWithVariables(ctx, facts, vars)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("direct rollout condition:", matched.Matched)

	engine := condition.NewEngine(condition.WithExpressionOptions(condition.WithVariables(vars)))
	if err := engine.AddRuleSet(featureRules()); err != nil {
		log.Fatal(err)
	}
	result, err := engine.Evaluate(ctx, facts, "feature-ai-workflows")
	if err != nil {
		log.Fatal(err)
	}
	printJSON("feature decision", result)
}

func featureRules() condition.RuleSet {
	return condition.RuleSet{
		Name: "feature-ai-workflows",
		Rules: []condition.Rule{
			{
				ID:          "deny-list",
				Priority:    100,
				Condition:   `account.id in lists.deny_accounts`,
				Decision:    "disabled",
				Actions:     []condition.Action{{Type: "flag_decision", Payload: map[string]any{"reason": "deny_list"}}},
				StopOnMatch: true,
			},
			{
				ID:          "allow-list",
				Priority:    90,
				Condition:   `account.id in lists.allow_accounts`,
				Decision:    "enabled",
				Actions:     []condition.Action{{Type: "flag_decision", Payload: map[string]any{"reason": "allow_list"}}},
				StopOnMatch: true,
			},
			{
				ID:       "rollout",
				Priority: 50,
				Decision: "enabled",
				Actions:  []condition.Action{{Type: "flag_decision", Payload: map[string]any{"reason": "percentage_rollout"}}},
				Group: &condition.Group{Mode: condition.GroupAll, Rules: []condition.Rule{
					{ID: "active", Condition: `account.status == "active"`},
					{ID: "tier", Condition: `account.tier in ["enterprise", "business"]`},
					{ID: "seat-range", Condition: `account.seat_count >= feature.min_seats and account.seat_count <= $maxSeats`},
					{ID: "region-or-admin", Group: &condition.Group{Mode: condition.GroupAny, Rules: []condition.Rule{
						{ID: "region", Condition: `account.region in ["US", "CA", "GB"]`},
						{ID: "admin", Condition: `user.role == "admin"`},
					}}},
					{ID: "bucket", Condition: `stableBucket(account.id, $bucketModulo) < feature.rollout_percent`},
				}},
				StopOnMatch: true,
			},
		},
	}
}

func printJSON(label string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s:\n%s\n", label, data)
}
