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
		"user": map[string]any{
			"id": "buyer_123", "account_age_days": 45, "chargeback_count": 0, "trust_score": 78,
			"recent_orders": []map[string]any{
				{"status": "paid", "amount": 120},
				{"status": "paid", "amount": 80},
				{"status": "cancelled", "amount": 40},
			},
		},
		"device": map[string]any{"id": "dev_9", "reputation": 0.82, "proxy": false},
		"order":  map[string]any{"id": "ord_77", "total": 420, "risk_score": 0.46, "shipping_country": "US", "billing_country": "US"},
		"cart": map[string]any{"items": []map[string]any{
			{"sku": "phone", "category": "electronics", "amount": 349, "risk": 0.28},
			{"sku": "case", "category": "accessories", "amount": 29, "risk": 0.05},
			{"sku": "gift-card", "category": "gift_card", "amount": 42, "risk": 0.55},
		}},
	}
	vars := condition.MapFacts{"declineRisk": 0.85, "reviewRisk": 0.40, "minDeviceReputation": 0.70}

	expr := condition.MustCompile(`
		order.risk_score < $declineRisk and
		device.reputation >= $minDeviceReputation and
		count(filter(cart.items, "category", "==", "gift_card")) <= 1 and
		avg(cart.items, "risk") < 0.40
	`)
	ok, err := expr.EvalWithVariables(ctx, facts, vars)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("direct fraud guard:", ok.Matched)

	engine := condition.NewEngine(condition.WithExpressionOptions(condition.WithVariables(vars)))
	if err := engine.AddRuleSet(fraudRules()); err != nil {
		log.Fatal(err)
	}
	result, err := engine.Evaluate(ctx, facts, "fraud-review")
	if err != nil {
		log.Fatal(err)
	}
	printJSON("fraud decision", result)
}

func fraudRules() condition.RuleSet {
	return condition.RuleSet{
		Name: "fraud-review",
		Rules: []condition.Rule{
			{
				ID:       "decline",
				Priority: 100,
				Decision: "decline",
				Actions:  []condition.Action{{Type: "block_order", Payload: map[string]any{"reason": "high_risk"}}},
				Group: &condition.Group{Mode: condition.GroupAny, Rules: []condition.Rule{
					{ID: "risk-score", Condition: `order.risk_score >= $declineRisk`},
					{ID: "proxy-new-account", Condition: `device.proxy == true and user.account_age_days < 7`},
					{ID: "chargebacks", Condition: `defaultValue(user.chargeback_count, 0) >= 2`},
				}},
				StopOnMatch: true,
			},
			{
				ID:       "manual-review",
				Priority: 50,
				Decision: "review",
				Actions:  []condition.Action{{Type: "queue_review", Payload: map[string]any{"queue": "fraud_ops"}}},
				Group: &condition.Group{Mode: condition.GroupAny, Rules: []condition.Rule{
					{ID: "review-risk-score", Condition: `order.risk_score >= $reviewRisk`},
					{ID: "country-mismatch", Condition: `order.shipping_country != order.billing_country`},
					{ID: "risky-cart", Group: &condition.Group{Mode: condition.GroupAll, Rules: []condition.Rule{
						{ID: "high-value", Condition: `order.total >= 300`},
						{ID: "cart-risk", Condition: `avg(cart.items, "risk") >= 0.25`},
					}}},
					{ID: "velocity", Condition: `countWhere(user.recent_orders, "status", "paid") >= 3`},
				}},
				StopOnMatch: true,
			},
			{
				ID:       "approve",
				Priority: 10,
				Decision: "approve",
				Actions:  []condition.Action{{Type: "capture_payment"}},
				Group: &condition.Group{Mode: condition.GroupAll, Rules: []condition.Rule{
					{ID: "trusted-user", Condition: `user.trust_score >= 60`},
					{ID: "device-ok", Condition: `device.reputation >= $minDeviceReputation`},
					{ID: "not-proxy", Group: &condition.Group{Mode: condition.GroupNone, Rules: []condition.Rule{
						{ID: "proxy", Condition: `device.proxy == true`},
					}}},
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
