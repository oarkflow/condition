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
			"tenant":    map[string]any{"id": "acme", "plan": "enterprise"},
			"message":   map[string]any{"type": "transactional", "category": "receipt", "priority": "high"},
			"recipient": map[string]any{"email": "buyer@example.com", "domain": "example.com", "region": "us-east", "suppressed": false},
			"campaign":  map[string]any{"complaint_rate": 0.002, "bounce_rate": 0.006},
		},
		Variables: condition.MapFacts{"maxComplaintRate": 0.005, "maxLatencyMS": 900},
		Candidates: []condition.Candidate{
			emailProvider("ses-us-east", "AWS SES us-east", true, []string{"us-east", "us-west"}, []string{"transactional", "marketing"}, []string{}, 0.992, 180, 0.00010, 100, 4, 1, 60),
			emailProvider("sendgrid-global", "SendGrid Global", true, []string{"us-east", "eu-west", "ap-south"}, []string{"transactional", "marketing"}, []string{"blocked.example"}, 0.989, 260, 0.00018, 95, 3, 2, 30),
			emailProvider("mailgun-eu", "Mailgun EU", true, []string{"eu-west"}, []string{"transactional"}, []string{}, 0.985, 340, 0.00016, 80, 3, 3, 10),
			emailProvider("postmark-txn", "Postmark Transactional", false, []string{"us-east"}, []string{"transactional"}, []string{}, 0.998, 140, 0.00035, 100, 6, 1, 10),
		},
	}

	ranker, err := condition.NewRanker(
		emailRules(),
		condition.WithRoutingTieBreakerPaths("provider.priority", "provider.specificity", "provider.fallback_order", "provider.cost_per_email"),
		condition.WithWeightedSelection("provider.weight", "recipient.email"),
	)
	if err != nil {
		log.Fatal(err)
	}
	printRanking(ctx, ranker, req)
}

func emailRules() condition.RuleSet {
	return condition.RuleSet{
		Name: "email-provider-routing",
		Rules: []condition.Rule{
			{ID: "provider-active", Condition: `provider.active == true`, Reason: "provider is disabled"},
			{ID: "not-suppressed", Condition: `recipient.suppressed == false`, Reason: "recipient is suppressed"},
			{ID: "region", Condition: `recipient.region in provider.regions`, Reason: "recipient region is unsupported"},
			{ID: "message-type", Condition: `message.type in provider.message_types`, Reason: "message type is unsupported"},
			{ID: "domain-block", Group: &condition.Group{Mode: condition.GroupNone, Rules: []condition.Rule{
				{ID: "blocked-domain", Condition: `recipient.domain in provider.blocked_domains`, Reason: "recipient domain is blocked"},
			}}},
			{ID: "campaign-health", Group: &condition.Group{Mode: condition.GroupAll, Rules: []condition.Rule{
				{ID: "complaints", Condition: `campaign.complaint_rate <= $maxComplaintRate`},
				{ID: "latency", Condition: `provider.latency_ms <= $maxLatencyMS`},
			}}},
		},
		ScoreRules: []condition.ScoreRule{
			{ID: "deliverability", Metric: "provider.deliverability", Weight: 0.55, Direction: condition.HigherBetter, Normalize: condition.Normalize{Min: 0.95, Max: 1}},
			{ID: "latency", Metric: "provider.latency_ms", Weight: 0.25, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 100, Max: 1000}},
			{ID: "cost", Metric: "provider.cost_per_email", Weight: 0.20, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 0.00005, Max: 0.00040}},
		},
	}
}

func emailProvider(id, name string, active bool, regions, types, blocked []string, deliverability float64, latency int, cost float64, capacity, priority, fallback, weight int) condition.Candidate {
	return condition.Candidate{ID: id, Name: name, Facts: condition.MapFacts{
		"active": active, "regions": regions, "message_types": types, "blocked_domains": blocked,
		"deliverability": deliverability, "latency_ms": latency, "cost_per_email": cost,
		"capacity": capacity, "priority": priority, "fallback_order": fallback, "weight": weight,
		"specificity": len(regions) + len(types),
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
