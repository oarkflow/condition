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
	facts := checkoutFacts()
	vars := condition.MapFacts{
		"minTrustScore":      70,
		"highRiskThreshold":  0.65,
		"freeShippingFloor":  100,
		"maxDeliveryDays":    3,
		"minProviderQuality": 0.92,
	}

	direct := condition.MustCompile(`
		user.age >= 18 and
		defaultValue(user.chargeback_count, 0) <= 1 and
		count(filter(cart.items, "category", "==", "electronics")) >= 1 and
		sum(cart.items, "price") >= $freeShippingFloor and
		avg(cart.items, "risk") < $highRiskThreshold and
		none(cart.items, "status", "==", "restricted") and
		betweenTime(order.created_at, "2026-01-01T00:00:00Z", "2026-12-31T23:59:59Z")
	`)
	directResult, err := direct.EvalWithVariables(ctx, facts, vars)
	if err != nil {
		log.Fatal(err)
	}
	printJSON("direct checkout condition", directResult)

	engine := condition.NewEngine(condition.WithExpressionOptions(condition.WithVariables(vars)))
	if err := engine.AddRuleSet(checkoutRuleSet()); err != nil {
		log.Fatal(err)
	}
	decision, err := engine.Evaluate(ctx, facts, "marketplace-checkout")
	if err != nil {
		log.Fatal(err)
	}
	printJSON("nested checkout rules", decision)

	ranker, err := condition.NewRanker(
		fulfillmentRuleSet(),
		condition.WithRankerExplain(false),
		condition.WithRoutingTieBreakerPaths(
			"provider.priority",
			"provider.specificity",
			"provider.fallback_order",
			"provider.cost",
		),
		condition.WithWeightedSelection("provider.weight", "user.id"),
	)
	if err != nil {
		log.Fatal(err)
	}
	req := condition.RankingRequest{
		Facts:      facts,
		Variables:  vars,
		Candidates: fulfillmentProviders(),
	}
	ranked, err := ranker.Rank(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	if ranked.Winner != nil {
		fmt.Printf("selected provider: %s (score %.4f)\n", ranked.Winner.ID, ranked.Winner.Score)
	}
	printJSON("provider ranking", ranked)

	fallbacks, err := ranker.SelectFallbacks(ctx, req, 2)
	if err != nil {
		log.Fatal(err)
	}
	printJSON("fallback providers", fallbacks)

	weighted, ok, err := ranker.SelectWeightedID(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	if ok {
		printJSON("stable weighted provider", weighted)
	}
}

func checkoutRuleSet() condition.RuleSet {
	return condition.RuleSet{
		Name: "marketplace-checkout",
		Rules: []condition.Rule{
			{
				ID:       "manual-review",
				Priority: 100,
				Decision: "review",
				Actions:  []condition.Action{{Type: "queue_review", Payload: map[string]any{"team": "risk"}}},
				Group: &condition.Group{Mode: condition.GroupAny, Rules: []condition.Rule{
					{ID: "risk-score", Condition: `order.risk_score >= $highRiskThreshold`, Reason: "order risk is high"},
					{ID: "velocity", Condition: `countWhere(user.recent_orders, "status", "paid") > 5`, Reason: "buyer velocity is high"},
					{ID: "risky-cart", Group: &condition.Group{Mode: condition.GroupAll, Rules: []condition.Rule{
						{ID: "large-order", Condition: `sum(cart.items, "price") >= 500`},
						{ID: "risky-items", Condition: `avg(cart.items, "risk") >= 0.30`},
					}}},
				}},
			},
			{
				ID:       "approve-checkout",
				Priority: 50,
				Decision: "approve",
				Score:    10,
				Actions:  []condition.Action{{Type: "authorize_payment"}},
				Group: &condition.Group{Mode: condition.GroupAll, Rules: []condition.Rule{
					{ID: "adult-buyer", Condition: `user.age >= 18`},
					{ID: "trusted-buyer", Condition: `user.trust_score >= $minTrustScore`},
					{ID: "no-restricted-items", Group: &condition.Group{Mode: condition.GroupNone, Rules: []condition.Rule{
						{ID: "restricted-item", Condition: `any(cart.items, "status", "==", "restricted")`},
					}}},
					{ID: "deliverable", Group: &condition.Group{Mode: condition.GroupAny, Rules: []condition.Rule{
						{ID: "domestic", Condition: `shipping.country == user.country`},
						{ID: "cross-border-ok", Condition: `shipping.cross_border == true and user.kyc_level >= 2`},
					}}},
				}},
			},
			{
				ID:       "free-shipping",
				Priority: 10,
				Decision: "discount",
				Actions:  []condition.Action{{Type: "apply_discount", Payload: map[string]any{"code": "FASTSHIP"}}},
				Condition: `
					sum(cart.items, "price") >= $freeShippingFloor and
					coalesce(user.membership, "standard") in ["gold", "platinum"]
				`,
			},
		},
	}
}

func fulfillmentRuleSet() condition.RuleSet {
	return condition.RuleSet{
		Name: "fulfillment-provider",
		Rules: []condition.Rule{
			{ID: "active", Condition: `provider.active == true`, Reason: "provider is inactive"},
			{ID: "country", Condition: `shipping.country in provider.countries`, Reason: "country is not supported"},
			{ID: "capacity", Condition: `cart.item_count <= provider.capacity`, Reason: "capacity is too low"},
			{ID: "delivery-window", Condition: `provider.delivery_days <= $maxDeliveryDays`, Reason: "delivery is too slow"},
			{ID: "category-support", Group: &condition.Group{Mode: condition.GroupAll, Rules: []condition.Rule{
				{ID: "electronics", Condition: `"electronics" in provider.categories`},
				{ID: "fragile", Condition: `cart.has_fragile == false or "fragile" in provider.handling`},
			}}},
			{ID: "not-blocked", Group: &condition.Group{Mode: condition.GroupNone, Rules: []condition.Rule{
				{ID: "blocked-postal", Condition: `shipping.postal_code in provider.blocked_postal_codes`, Reason: "postal code is blocked"},
			}}},
		},
		ScoreRules: []condition.ScoreRule{
			{ID: "quality", Condition: `provider.quality >= $minProviderQuality`, Metric: "provider.quality", Weight: 0.50, Direction: condition.HigherBetter, Normalize: condition.Normalize{Min: 0.90, Max: 1}},
			{ID: "speed", Metric: "provider.delivery_days", Weight: 0.25, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 1, Max: 5}},
			{ID: "cost", Metric: "provider.cost", Weight: 0.25, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 4, Max: 20}},
		},
	}
}

func checkoutFacts() condition.MapFacts {
	return condition.MapFacts{
		"user": map[string]any{
			"id": "buyer-1001", "age": 34, "country": "US", "trust_score": 86,
			"kyc_level": 2, "membership": "gold", "chargeback_count": 0,
			"recent_orders": []map[string]any{
				{"status": "paid", "total": 82},
				{"status": "paid", "total": 120},
				{"status": "refunded", "total": 30},
			},
		},
		"order": map[string]any{
			"id": "order-9001", "created_at": "2026-05-10T08:30:00Z", "risk_score": 0.24,
		},
		"cart": map[string]any{
			"item_count": 3, "has_fragile": true,
			"items": []map[string]any{
				{"sku": "phone-1", "category": "electronics", "price": 699.00, "risk": 0.20, "status": "active"},
				{"sku": "case-1", "category": "accessories", "price": 29.00, "risk": 0.05, "status": "active"},
				{"sku": "charger-1", "category": "electronics", "price": 39.00, "risk": 0.08, "status": "active"},
			},
		},
		"shipping": map[string]any{
			"country": "US", "postal_code": "94105", "cross_border": false,
		},
	}
}

func fulfillmentProviders() []condition.Candidate {
	return []condition.Candidate{
		provider("fast-west", "Fast West", true, []string{"US", "CA"}, []string{"electronics", "accessories"}, []string{"fragile"}, nil, 3, 0.98, 10, 100, 4, 1, 60),
		provider("budget-ground", "Budget Ground", true, []string{"US"}, []string{"electronics", "accessories"}, []string{"fragile"}, nil, 5, 0.94, 5, 70, 2, 3, 20),
		provider("premium-air", "Premium Air", true, []string{"US", "NP"}, []string{"electronics", "accessories"}, []string{"fragile"}, nil, 1, 0.99, 18, 110, 5, 2, 20),
		provider("blocked-local", "Blocked Local", true, []string{"US"}, []string{"electronics"}, []string{"fragile"}, []string{"94105"}, 1, 1.00, 4, 100, 9, 1, 10),
	}
}

func provider(id, name string, active bool, countries, categories, handling, blocked []string, days int, quality, cost float64, capacity, priority, fallback, weight int) condition.Candidate {
	return condition.Candidate{ID: id, Name: name, Facts: condition.MapFacts{
		"active": active, "countries": countries, "categories": categories, "handling": handling,
		"blocked_postal_codes": blocked, "delivery_days": days, "quality": quality, "cost": cost,
		"capacity": capacity, "priority": priority, "specificity": len(countries) + len(categories) + len(handling),
		"fallback_order": fallback, "weight": weight,
	}}
}

func printJSON(label string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%s:\n%s\n", label, b)
}
