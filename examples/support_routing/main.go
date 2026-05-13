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
			"customer": map[string]any{"id": "cust_500", "tier": "enterprise", "language": "en"},
			"ticket":   map[string]any{"id": "t_100", "topic": "billing", "priority": "urgent", "sla_minutes": 60, "sentiment": -0.4},
		},
		Variables: condition.MapFacts{"maxQueueDepth": 40, "minAvailability": 0.65},
		Candidates: []condition.Candidate{
			supportQueue("enterprise-billing", "Enterprise Billing", true, []string{"enterprise"}, []string{"billing", "payments"}, []string{"en", "es"}, 0.92, 12, 22, 5, 1, 70),
			supportQueue("general-billing", "General Billing", true, []string{"free", "business", "enterprise"}, []string{"billing"}, []string{"en"}, 0.84, 25, 40, 3, 2, 20),
			supportQueue("urgent-swat", "Urgent SWAT", true, []string{"enterprise"}, []string{"billing", "incident"}, []string{"en"}, 0.70, 8, 15, 4, 3, 10),
			supportQueue("offline-specialist", "Offline Specialist", false, []string{"enterprise"}, []string{"billing"}, []string{"en"}, 1.00, 1, 5, 9, 1, 10),
		},
	}

	ranker, err := condition.NewRanker(
		supportRules(),
		condition.WithRoutingTieBreakerPaths("provider.priority", "provider.specificity", "provider.fallback_order", "provider.queue_depth"),
		condition.WithWeightedSelection("provider.weight", "ticket.id"),
	)
	if err != nil {
		log.Fatal(err)
	}
	printRanking(ctx, ranker, req)
}

func supportRules() condition.RuleSet {
	return condition.RuleSet{
		Name: "support-routing",
		Rules: []condition.Rule{
			{ID: "available", Condition: `provider.online == true`, Reason: "queue is offline"},
			{ID: "tier", Condition: `customer.tier in provider.customer_tiers`, Reason: "customer tier is unsupported"},
			{ID: "skill", Condition: `ticket.topic in provider.skills`, Reason: "skill is unavailable"},
			{ID: "language", Condition: `customer.language in provider.languages`, Reason: "language is unsupported"},
			{ID: "queue-depth", Condition: `provider.queue_depth <= $maxQueueDepth`, Reason: "queue is overloaded"},
			{ID: "availability", Condition: `provider.availability >= $minAvailability`, Reason: "agent availability is too low"},
			{ID: "urgent-policy", Group: &condition.Group{Mode: condition.GroupAny, Rules: []condition.Rule{
				{ID: "normal-priority", Condition: `ticket.priority != "urgent"`},
				{ID: "urgent-supported", Condition: `"urgent" in provider.skills or customer.tier == "enterprise"`},
			}}},
		},
		ScoreRules: []condition.ScoreRule{
			{ID: "availability", Metric: "provider.availability", Weight: 0.45, Direction: condition.HigherBetter, Normalize: condition.Normalize{Min: 0.60, Max: 1}},
			{ID: "queue-depth", Metric: "provider.queue_depth", Weight: 0.35, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 0, Max: 50}},
			{ID: "sla", Metric: "provider.avg_resolution_minutes", Weight: 0.20, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 10, Max: 120}},
		},
	}
}

func supportQueue(id, name string, online bool, tiers, skills, languages []string, availability float64, queueDepth, resolution, priority, fallback, weight int) condition.Candidate {
	return condition.Candidate{ID: id, Name: name, Facts: condition.MapFacts{
		"online": online, "customer_tiers": tiers, "skills": skills, "languages": languages,
		"availability": availability, "queue_depth": queueDepth, "avg_resolution_minutes": resolution,
		"priority": priority, "fallback_order": fallback, "weight": weight,
		"specificity": len(tiers) + len(skills) + len(languages),
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
