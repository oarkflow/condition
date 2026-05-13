package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func main() {
	ranker, err := condition.NewRanker(
		smsRoutingRules(),
		condition.WithRankerTrace(true),
		condition.WithRoutingTieBreakerPaths(
			"route.priority",
			"route.specificity",
			"route.fallback_order",
			"quality.cost_per_sms",
		),
		condition.WithWeightedSelection("route.weight", "recipient.number"),
	)
	if err != nil {
		log.Fatal(err)
	}

	req := condition.RankingRequest{
		Facts: condition.MapFacts{
			"user": map[string]any{
				"id":        15,
				"tenant_id": 1001,
			},
			"message": map[string]any{
				"category":  "otp",
				"type":      "sms",
				"quality":   "critical",
				"count":     500,
				"body":      "Your OTP is 123456",
				"sender_id": "ACME",
			},
			"recipient": map[string]any{
				"number":  "+9779800000000",
				"country": "NP",
				"carrier": "NCELL",
			},
		},
		Candidates: smsRouteRows(),
	}

	result, err := ranker.Rank(context.Background(), req)
	if err != nil {
		log.Fatal(err)
	}
	if result.Winner != nil {
		fmt.Printf("selected route: %s via %s (score %.4f)\n\n", result.Winner.ID, result.Winner.Name, result.Winner.Score)
	}
	fallbacks, err := ranker.SelectFallbacks(context.Background(), req, 3)
	if err != nil {
		log.Fatal(err)
	}
	if fallbacks.Primary != nil {
		fmt.Printf("fallback chain: primary=%s\n", fallbacks.Primary.ID)
		for i, fallback := range fallbacks.Fallbacks {
			fmt.Printf("  %d. %s via %s\n", i+1, fallback.ID, fallback.Name)
		}
		fmt.Println()
	}
	weighted, ok, err := ranker.SelectWeightedID(context.Background(), req)
	if err != nil {
		log.Fatal(err)
	}
	if ok {
		fmt.Printf("weighted stable selection: %s via %s\n\n", weighted.ID, weighted.Name)
	}
	printJSON("ranking result", result)
}

func smsRoutingRules() condition.RuleSet {
	return condition.RuleSet{
		Name: "sms-route-table",
		Rules: []condition.Rule{
			{ID: "route-active", Condition: `active(route.status)`, Reason: "route is not active"},
			{ID: "provider-active", Condition: `active(provider.status)`, Reason: "provider is not active"},
			{ID: "valid-window", Condition: `validNow(route.valid_from, route.valid_to)`, Reason: "route is outside its validity window"},
			{ID: "tenant", Condition: `nullableMatch(route.tenant_id, user.tenant_id)`, Reason: "tenant does not match"},
			{ID: "user", Condition: `nullableMatch(route.user_id, user.id)`, Reason: "user does not match"},
			{ID: "category", Condition: `nullableMatch(route.message_category, message.category)`, Reason: "message category does not match"},
			{ID: "type", Condition: `nullableMatch(route.message_type, message.type)`, Reason: "message type does not match"},
			{ID: "quality", Condition: `nullableMatch(route.delivery_quality, message.quality)`, Reason: "delivery quality does not match"},
			{ID: "country", Condition: `nullableMatch(route.recipient_country, recipient.country)`, Reason: "recipient country does not match"},
			{ID: "carrier", Condition: `nullableMatch(route.carrier_code, recipient.carrier)`, Reason: "carrier does not match"},
			{ID: "message-count", Condition: `rangeMatch(route.min_message_count, route.max_message_count, message.count)`, Reason: "message count is outside route bounds"},
			{ID: "pattern", Condition: `route.message_pattern == null or regex_match(message.body, route.message_pattern)`, Reason: "message body pattern does not match"},
			{ID: "sender", Condition: `nullableMatch(route.sender_id, message.sender_id)`, Reason: "sender id does not match"},
		},
		ScoreRules: []condition.ScoreRule{
			{ID: "success-rate", Metric: "quality.success_rate", Weight: 0.60, Direction: condition.HigherBetter, Normalize: condition.Normalize{Min: 0.90, Max: 1}, Reason: "higher delivery success rate improves score"},
			{ID: "delivery-speed", Metric: "quality.avg_delivery_seconds", Weight: 0.20, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 1, Max: 15}, Reason: "faster delivery improves score"},
			{ID: "failure-rate", Metric: "quality.failure_rate", Weight: 0.10, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 0, Max: 0.10}, Reason: "lower failure rate improves score"},
			{ID: "cost", Metric: "quality.cost_per_sms", Weight: 0.10, Direction: condition.LowerBetter, Normalize: condition.Normalize{Min: 0.003, Max: 0.03}, Reason: "lower cost improves score"},
		},
	}
}

func smsRouteRows() []condition.Candidate {
	return []condition.Candidate{
		routeCandidate("r-user-otp-np-ncell", "Sparrow SMS", routeRow{
			UserID: 15, TenantID: 1001, Category: "otp", Type: "sms", Quality: "critical",
			Country: "NP", Carrier: "NCELL", MinCount: 1, MaxCount: 1000,
			Pattern: "OTP", SenderID: "ACME", Priority: 100, Fallback: 1, Weight: 70,
		}, providerRow{ID: 1, Code: "sparrow", Status: "active"}, qualityRow{
			SuccessRate: 0.991, AvgDeliverySeconds: 2.0, FailureRate: 0.004, CostPerSMS: 0.010,
		}),
		routeCandidate("r-user-otp-np", "Provider B", routeRow{
			UserID: 15, TenantID: 1001, Category: "otp", Type: "sms", Quality: "critical",
			Country: "NP", MinCount: 1, MaxCount: 2000,
			Priority: 95, Fallback: 2, Weight: 30,
		}, providerRow{ID: 2, Code: "provider_b", Status: "active"}, qualityRow{
			SuccessRate: 0.985, AvgDeliverySeconds: 3.0, FailureRate: 0.006, CostPerSMS: 0.008,
		}),
		routeCandidate("r-user-default", "Provider C", routeRow{
			UserID: 15, TenantID: 1001, Priority: 80, Fallback: 3,
		}, providerRow{ID: 3, Code: "provider_c", Status: "active"}, qualityRow{
			SuccessRate: 0.970, AvgDeliverySeconds: 5.0, FailureRate: 0.012, CostPerSMS: 0.007,
		}),
		routeCandidate("r-global-otp-np", "Global OTP Provider", routeRow{
			Category: "otp", Country: "NP", Priority: 70, Fallback: 4,
		}, providerRow{ID: 4, Code: "global_otp", Status: "active"}, qualityRow{
			SuccessRate: 0.982, AvgDeliverySeconds: 4.0, FailureRate: 0.008, CostPerSMS: 0.006,
		}),
		routeCandidate("r-global-default", "Global Default Provider", routeRow{
			Default: true, Priority: 10, Fallback: 99,
		}, providerRow{ID: 5, Code: "global_default", Status: "active"}, qualityRow{
			SuccessRate: 0.940, AvgDeliverySeconds: 9.0, FailureRate: 0.030, CostPerSMS: 0.004,
		}),
		routeCandidate("r-inactive-provider", "Inactive Provider", routeRow{
			UserID: 15, Category: "otp", Country: "NP", Carrier: "NCELL", Priority: 999, Fallback: 1,
		}, providerRow{ID: 6, Code: "inactive", Status: "inactive"}, qualityRow{
			SuccessRate: 1.0, AvgDeliverySeconds: 1.0, FailureRate: 0, CostPerSMS: 0.001,
		}),
	}
}

type routeRow struct {
	UserID   any
	TenantID any
	Category string
	Type     string
	Quality  string
	Country  string
	Carrier  string
	MinCount any
	MaxCount any
	Pattern  string
	SenderID string
	Priority int
	Fallback int
	Weight   int
	Default  bool
}

type providerRow struct {
	ID     int
	Code   string
	Status string
}

type qualityRow struct {
	SuccessRate        float64
	AvgDeliverySeconds float64
	FailureRate        float64
	CostPerSMS         float64
}

func routeCandidate(id, providerName string, route routeRow, provider providerRow, quality qualityRow) condition.Candidate {
	routeMap := condition.MapFacts{
		"id":                id,
		"user_id":           nullable(route.UserID),
		"tenant_id":         nullable(route.TenantID),
		"message_category":  nullableString(route.Category),
		"message_type":      nullableString(route.Type),
		"delivery_quality":  nullableString(route.Quality),
		"recipient_country": nullableString(route.Country),
		"carrier_code":      nullableString(route.Carrier),
		"min_message_count": nullable(route.MinCount),
		"max_message_count": nullable(route.MaxCount),
		"message_pattern":   nullableString(route.Pattern),
		"sender_id":         nullableString(route.SenderID),
		"priority":          route.Priority,
		"fallback_order":    route.Fallback,
		"weight":            route.Weight,
		"is_default":        route.Default,
		"status":            "active",
		"valid_from":        nil,
		"valid_to":          nil,
	}
	routeMap["specificity"] = routeSpecificity(routeMap)
	return condition.Candidate{
		ID:   id,
		Name: providerName,
		Facts: condition.MapFacts{
			"route": routeMap,
			"provider": condition.MapFacts{
				"id":     provider.ID,
				"name":   providerName,
				"code":   provider.Code,
				"status": provider.Status,
			},
			"quality": condition.MapFacts{
				"success_rate":         quality.SuccessRate,
				"avg_delivery_seconds": quality.AvgDeliverySeconds,
				"failure_rate":         quality.FailureRate,
				"cost_per_sms":         quality.CostPerSMS,
			},
		},
	}
}

func routeSpecificity(route condition.MapFacts) int {
	score := 0
	weights := map[string]int{
		"user_id": 10, "message_category": 8, "message_type": 6,
		"delivery_quality": 6, "recipient_country": 5, "carrier_code": 5,
		"min_message_count": 3, "max_message_count": 3, "message_pattern": 4,
	}
	for key, weight := range weights {
		if route[key] != nil {
			score += weight
		}
	}
	return score
}

func nullable(v any) any {
	switch x := v.(type) {
	case int:
		if x == 0 {
			return nil
		}
	case string:
		if x == "" {
			return nil
		}
	}
	return v
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func printJSON(title string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s:\n%s\n", title, data)
}
