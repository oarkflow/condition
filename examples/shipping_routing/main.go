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
	req := condition.RankingRequest{
		Facts: condition.MapFacts{
			"merchant": map[string]any{"id": "shop_77", "tier": "plus"},
			"shipment": map[string]any{"country": "US", "postal_code": "94105", "weight_kg": 3.2, "value": 899, "fragile": true, "hazmat": false, "max_delivery_days": 3},
		},
		Variables: condition.MapFacts{"maxCost": 18.00, "minOnTimeRate": 0.94},
		Candidates: []condition.Candidate{
			shippingService("ups-2day", "UPS 2 Day", true, []string{"US", "CA"}, []string{"fragile"}, []string{}, 20, 2, 0.971, 16.80, 100, 5, 1, 50),
			shippingService("fedex-ground", "FedEx Ground", true, []string{"US"}, []string{"fragile"}, []string{}, 30, 3, 0.955, 9.40, 100, 3, 3, 20),
			shippingService("dhl-express", "DHL Express", true, []string{"US", "GB", "NP"}, []string{"fragile"}, []string{}, 10, 1, 0.980, 22.00, 80, 4, 2, 20),
			shippingService("local-blocked", "Local Courier", true, []string{"US"}, []string{"fragile"}, []string{"94105"}, 10, 1, 0.990, 8.00, 90, 9, 1, 10),
		},
	}

	ranker, err := condition.NewRanker(
		shippingRules(),
		condition.WithRoutingTieBreakerPaths("provider.priority", "provider.specificity", "provider.fallback_order", "provider.cost"),
		condition.WithWeightedSelection("provider.weight", "merchant.id"),
	)
	if err != nil {
		log.Fatal(err)
	}
	printRanking(ctx, ranker, req)
}

func shippingRules() condition.RuleSet {
	return condition.RuleSet{
		Name: "shipping-carrier-routing",
		Rules: []condition.Rule{
			{ID: "active", Condition: `provider.active == true`, Reason: "carrier is disabled"},
			{ID: "country", Condition: `shipment.country in provider.countries`, Reason: "destination country is unsupported"},
			{ID: "weight", Condition: `shipment.weight_kg <= provider.max_weight_kg`, Reason: "package is too heavy"},
			{ID: "sla", Condition: `provider.delivery_days <= shipment.max_delivery_days`, Reason: "delivery SLA is too slow"},
			{ID: "cost-cap", Condition: `provider.cost <= $maxCost`, Reason: "shipping cost exceeds cap"},
			{ID: "service-quality", Condition: `provider.on_time_rate >= $minOnTimeRate`, Reason: "on-time rate is below threshold"},
			{ID: "handling", Group: &condition.Group{Mode: condition.GroupAll, Rules: []condition.Rule{
				{ID: "fragile", Condition: `shipment.fragile == false or "fragile" in provider.handling`},
				{ID: "hazmat", Condition: `shipment.hazmat == false or "hazmat" in provider.handling`},
			}}},
			{ID: "postal-block", Group: &condition.Group{Mode: condition.GroupNone, Rules: []condition.Rule{
				{ID: "blocked-postal", Condition: `shipment.postal_code in provider.blocked_postal_codes`, Reason: "postal code is blocked"},
			}}},
		},
		ScoreRules: []condition.ScoreRule{
			{ID: "on-time", Metric: "provider.on_time_rate", Weight: 0.45, Direction: condition.HigherBetter, Normalize: condition.Normalize{Min: 0.90, Max: 1}},
			{ID: "speed", Metric: "provider.delivery_days", Weight: 0.30, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 1, Max: 5}},
			{ID: "cost", Metric: "provider.cost", Weight: 0.25, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 5, Max: 25}},
		},
	}
}

func shippingService(id, name string, active bool, countries, handling, blocked []string, maxWeight, days int, onTime, cost float64, capacity, priority, fallback, weight int) condition.Candidate {
	return condition.Candidate{ID: id, Name: name, Facts: condition.MapFacts{
		"active": active, "countries": countries, "handling": handling, "blocked_postal_codes": blocked,
		"max_weight_kg": maxWeight, "delivery_days": days, "on_time_rate": onTime, "cost": cost,
		"capacity": capacity, "priority": priority, "fallback_order": fallback, "weight": weight,
		"specificity": len(countries) + len(handling),
	}}
}

func printRanking(ctx context.Context, ranker *condition.Ranker, req condition.RankingRequest) {
	result, err := ranker.Rank(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	if result.Winner != nil {
		fmt.Printf("winner: %s via %s (score %.4f)\n", result.Winner.ID, result.Winner.Name, result.Winner.Score)
	}
	fallbacks, err := ranker.SelectFallbacks(ctx, req, 2)
	if err != nil {
		log.Fatal(err)
	}
	selected, ok, err := ranker.SelectBestID(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	if ok {
		fmt.Printf("hot path selected: %s (score %.4f)\n", selected.ID, selected.Score)
	}
	weighted, ok, err := ranker.SelectWeightedID(ctx, req)
	if err != nil {
		log.Fatal(err)
	}
	if ok {
		fmt.Printf("weighted selected: %s\n", weighted.ID)
	}
	printJSON("ranking", result)
	printJSON("fallbacks", fallbacks)
}

func printJSON(label string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%s:\n%s\n", label, data)
}
