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
			"merchant": map[string]any{"id": "m_100", "mcc": "5732", "risk_tier": "standard"},
			"payment":  map[string]any{"amount": 249.90, "currency": "USD", "country": "US", "card_brand": "visa", "requires_3ds": false, "risk_score": 0.18},
			"customer": map[string]any{"id": "cus_42", "trusted": true},
		},
		Variables: condition.MapFacts{"maxRiskScore": 0.55, "maxFeeBps": 290},
		Candidates: []condition.Candidate{
			paymentProcessor("stripe-primary", "Stripe", true, []string{"USD", "EUR"}, []string{"US", "CA"}, []string{"visa", "mastercard", "amex"}, false, 0.982, 210, 1, 96, 5, 1, 70),
			paymentProcessor("adyen-us", "Adyen US", true, []string{"USD"}, []string{"US"}, []string{"visa", "mastercard"}, false, 0.976, 180, 2, 92, 4, 2, 20),
			paymentProcessor("worldpay-3ds", "Worldpay 3DS", true, []string{"USD", "GBP"}, []string{"US", "GB"}, []string{"visa", "mastercard"}, true, 0.965, 260, 1, 88, 3, 3, 10),
			paymentProcessor("offline-backup", "Offline Backup", false, []string{"USD"}, []string{"US"}, []string{"visa"}, false, 0.990, 150, 3, 99, 9, 1, 10),
		},
	}

	ranker, err := condition.NewRanker(
		paymentRules(),
		condition.WithRoutingTieBreakerPaths("provider.priority", "provider.specificity", "provider.fallback_order", "provider.fee_bps"),
		condition.WithWeightedSelection("provider.weight", "customer.id"),
	)
	if err != nil {
		log.Fatal(err)
	}
	printRanking(ctx, ranker, req)
}

func paymentRules() condition.RuleSet {
	return condition.RuleSet{
		Name: "payment-processor-routing",
		Rules: []condition.Rule{
			{ID: "active", Condition: `provider.active == true`, Reason: "processor is disabled"},
			{ID: "currency", Condition: `payment.currency in provider.currencies`, Reason: "currency is unsupported"},
			{ID: "country", Condition: `payment.country in provider.countries`, Reason: "country is unsupported"},
			{ID: "card-brand", Condition: `payment.card_brand in provider.card_brands`, Reason: "card brand is unsupported"},
			{ID: "risk", Condition: `payment.risk_score <= $maxRiskScore`, Reason: "payment risk is too high"},
			{ID: "fee-cap", Condition: `provider.fee_bps <= $maxFeeBps`, Reason: "processor fee exceeds cap"},
			{ID: "three-ds", Group: &condition.Group{Mode: condition.GroupAny, Rules: []condition.Rule{
				{ID: "not-required", Condition: `payment.requires_3ds == false`},
				{ID: "provider-supports-3ds", Condition: `provider.supports_3ds == true`},
			}}},
		},
		ScoreRules: []condition.ScoreRule{
			{ID: "approval-rate", Metric: "provider.approval_rate", Weight: 0.50, Direction: condition.HigherBetter, Normalize: condition.Normalize{Min: 0.90, Max: 1}},
			{ID: "fee", Metric: "provider.fee_bps", Weight: 0.30, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 100, Max: 320}},
			{ID: "settlement", Metric: "provider.settlement_days", Weight: 0.20, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 1, Max: 5}},
		},
	}
}

func paymentProcessor(id, name string, active bool, currencies, countries, brands []string, supports3DS bool, approval float64, feeBps, settlement, capacity, priority, fallback, weight int) condition.Candidate {
	return condition.Candidate{ID: id, Name: name, Facts: condition.MapFacts{
		"active": active, "currencies": currencies, "countries": countries, "card_brands": brands,
		"supports_3ds": supports3DS, "approval_rate": approval, "fee_bps": feeBps,
		"settlement_days": settlement, "capacity": capacity, "priority": priority,
		"fallback_order": fallback, "weight": weight, "specificity": len(currencies) + len(countries) + len(brands),
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
