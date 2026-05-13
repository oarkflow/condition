package condition

import (
	"context"
	"testing"
)

func TestSMSRouteTableSelectsUserExactRoute(t *testing.T) {
	ranker := mustRouteTableRanker(t)
	result, err := ranker.Rank(context.Background(), smsRouteRequest())
	if err != nil {
		t.Fatalf("Rank returned error: %v", err)
	}
	if result.Winner == nil || result.Winner.ID != "r-user-otp-np-ncell" {
		t.Fatalf("winner = %#v, want user exact route", result.Winner)
	}
}

func TestSMSRouteTableSelectBestIDNativePath(t *testing.T) {
	ranker := mustRouteTableRanker(t)
	selected, ok, err := ranker.SelectBestID(context.Background(), smsRouteRequest())
	if err != nil {
		t.Fatalf("SelectBestID returned error: %v", err)
	}
	if !ok || selected.ID != "r-user-otp-np-ncell" {
		t.Fatalf("selected = %#v, ok=%v; want exact route", selected, ok)
	}
}

func TestSMSRouteTableFallsBackThroughPartialAndGlobalDefaults(t *testing.T) {
	ranker := mustRouteTableRanker(t)
	req := smsRouteRequest()
	req.Facts = MapFacts{
		"user": map[string]any{"id": 99, "tenant_id": 1001},
		"message": map[string]any{
			"category": "alert", "type": "sms", "quality": "standard",
			"count": 20, "body": "System alert", "sender_id": "ACME",
		},
		"recipient": map[string]any{"number": "+14155550100", "country": "US", "carrier": "VERIZON"},
	}
	result, err := ranker.Rank(context.Background(), req)
	if err != nil {
		t.Fatalf("Rank returned error: %v", err)
	}
	if result.Winner == nil || result.Winner.ID != "r-global-default" {
		t.Fatalf("winner = %#v, want global default", result.Winner)
	}
}

func TestSMSRouteTableUsesQualityThenCostAfterPriorityAndSpecificity(t *testing.T) {
	ranker := mustRouteTableRanker(t)
	req := RankingRequest{
		Facts: MapFacts{
			"user":      map[string]any{"id": 15, "tenant_id": 1001},
			"message":   map[string]any{"category": "otp", "type": "sms", "quality": "critical", "count": 10, "body": "OTP 123", "sender_id": "ACME"},
			"recipient": map[string]any{"number": "+9779800000000", "country": "NP", "carrier": "NCELL"},
		},
		Candidates: []Candidate{
			smsRouteCandidate("same-a", "A", smsRouteRow{UserID: 15, Category: "otp", Country: "NP", Carrier: "NCELL", Priority: 10, Fallback: 2}, smsProviderRow{ID: 1, Status: "active"}, smsQualityRow{SuccessRate: 0.950, AvgDeliverySeconds: 4, FailureRate: 0.02, CostPerSMS: 0.010}),
			smsRouteCandidate("same-b", "B", smsRouteRow{UserID: 15, Category: "otp", Country: "NP", Carrier: "NCELL", Priority: 10, Fallback: 1}, smsProviderRow{ID: 2, Status: "active"}, smsQualityRow{SuccessRate: 0.990, AvgDeliverySeconds: 2, FailureRate: 0.004, CostPerSMS: 0.012}),
		},
	}
	result, err := ranker.Rank(context.Background(), req)
	if err != nil {
		t.Fatalf("Rank returned error: %v", err)
	}
	if result.Winner == nil || result.Winner.ID != "same-b" {
		t.Fatalf("winner = %#v, want higher quality same-specificity route", result.Winner)
	}
}

func TestSMSRouteTableRejectsInactiveProviderAndMessageBounds(t *testing.T) {
	ranker := mustRouteTableRanker(t)
	result, err := ranker.Rank(context.Background(), smsRouteRequest())
	if err != nil {
		t.Fatalf("Rank returned error: %v", err)
	}
	rejected := map[string]CandidateResult{}
	for _, c := range result.Candidates {
		if !c.Eligible {
			rejected[c.ID] = c
		}
	}
	if rejected["r-inactive-provider"].Reasons[0] != "provider is not active" {
		t.Fatalf("inactive reason = %#v", rejected["r-inactive-provider"].Reasons)
	}
	if rejected["r-too-small"].Reasons[0] != "message count is outside route bounds" {
		t.Fatalf("bounds reason = %#v", rejected["r-too-small"].Reasons)
	}
}

func TestRankerSelectFallbacks(t *testing.T) {
	ranker := mustRouteTableRanker(t)
	fallbacks, err := ranker.SelectFallbacks(context.Background(), smsRouteRequest(), 2)
	if err != nil {
		t.Fatalf("SelectFallbacks returned error: %v", err)
	}
	if fallbacks.Primary == nil || fallbacks.Primary.ID != "r-user-otp-np-ncell" {
		t.Fatalf("primary = %#v", fallbacks.Primary)
	}
	if len(fallbacks.Fallbacks) != 2 {
		t.Fatalf("fallback count = %d, want 2", len(fallbacks.Fallbacks))
	}
	if fallbacks.Fallbacks[0].ID != "r-user-otp-np" {
		t.Fatalf("first fallback = %s, want r-user-otp-np", fallbacks.Fallbacks[0].ID)
	}
}

func TestRankerWeightedSelectionIsStable(t *testing.T) {
	ranker, err := NewRanker(
		RuleSet{
			Name: "weighted",
			Rules: []Rule{
				{ID: "active", Condition: `active(route.status)`},
			},
		},
		WithRoutingTieBreakerPaths("route.priority", "route.specificity", "route.fallback_order", "quality.cost_per_sms"),
		WithWeightedSelection("route.weight", "user.id"),
	)
	if err != nil {
		t.Fatalf("NewRanker returned error: %v", err)
	}
	req := RankingRequest{
		Facts: MapFacts{"user": map[string]any{"id": "u-1"}},
		Candidates: []Candidate{
			{ID: "a", Facts: MapFacts{"route": MapFacts{"status": "active", "priority": 10, "specificity": 1, "fallback_order": 1, "weight": 70}}},
			{ID: "b", Facts: MapFacts{"route": MapFacts{"status": "active", "priority": 10, "specificity": 1, "fallback_order": 2, "weight": 30}}},
		},
	}
	first, ok, err := ranker.SelectWeightedID(context.Background(), req)
	if err != nil || !ok {
		t.Fatalf("SelectWeightedID = %#v, %v, %v", first, ok, err)
	}
	for i := 0; i < 10; i++ {
		next, ok, err := ranker.SelectWeightedID(context.Background(), req)
		if err != nil || !ok {
			t.Fatalf("SelectWeightedID repeat = %#v, %v, %v", next, ok, err)
		}
		if next.ID != first.ID {
			t.Fatalf("weighted selection changed from %s to %s", first.ID, next.ID)
		}
	}
}

func TestRankerMissingMetricStrictness(t *testing.T) {
	rules := RuleSet{
		Name: "missing",
		ScoreRules: []ScoreRule{
			{ID: "quality", Metric: "quality.success_rate", Weight: 1},
		},
	}
	permissive, err := NewRanker(rules)
	if err != nil {
		t.Fatalf("NewRanker permissive returned error: %v", err)
	}
	res, err := permissive.Rank(context.Background(), RankingRequest{Candidates: []Candidate{{ID: "p1"}}})
	if err != nil {
		t.Fatalf("Rank permissive returned error: %v", err)
	}
	if !res.Candidates[0].Eligible {
		t.Fatal("expected permissive missing metric to remain eligible")
	}
	strict, err := NewRanker(rules, WithRankerStrict(true))
	if err != nil {
		t.Fatalf("NewRanker strict returned error: %v", err)
	}
	res, err = strict.Rank(context.Background(), RankingRequest{Candidates: []Candidate{{ID: "p1"}}})
	if err != nil {
		t.Fatalf("Rank strict returned error: %v", err)
	}
	if res.Candidates[0].Eligible {
		t.Fatal("expected strict missing metric to make candidate ineligible")
	}
}

func TestRankerNativeFastPathMatchesSelectorsWithGroupsVariablesAndActions(t *testing.T) {
	ranker, err := NewRanker(
		RuleSet{
			Name: "native-fast",
			Rules: []Rule{
				{
					ID:        "market-open",
					Condition: `market.status == "open"`,
					Decision:  "eligible",
					Actions:   []Action{{Type: "route", Payload: map[string]any{"queue": "standard"}}},
					Score:     2,
					Group: &Group{Mode: GroupAll, Rules: []Rule{
						{ID: "tier", Condition: `user.tier in provider.tiers`},
						{ID: "capacity", Condition: `order.total <= provider.remaining_capacity`},
						{ID: "quality-or-trusted", Group: &Group{Mode: GroupAny, Rules: []Rule{
							{ID: "quality", Condition: `provider.quality >= $minQuality`},
							{ID: "trusted", Condition: `provider.trusted == true`},
						}}},
						{ID: "not-blocked", Group: &Group{Mode: GroupNone, Rules: []Rule{
							{ID: "blocked-country", Condition: `recipient.country in provider.blocked_countries`, Reason: "country is blocked"},
						}}},
					}},
				},
			},
			ScoreRules: []ScoreRule{
				{ID: "quality-score", Condition: `provider.quality >= $scoreFloor`, Metric: "provider.quality", Weight: 0.7, Normalize: Normalize{Min: 0.90, Max: 1}},
				{ID: "cost-score", Metric: "provider.cost", Weight: 0.3, Direction: LowerBetter, Normalize: Normalize{Min: 0.01, Max: 0.05}},
			},
		},
		WithRankerExplain(false),
		WithRoutingTieBreakerPaths("provider.priority", "provider.specificity", "provider.fallback_order", "provider.cost"),
	)
	if err != nil {
		t.Fatalf("NewRanker returned error: %v", err)
	}
	req := RankingRequest{
		Facts: MapFacts{
			"market":    map[string]any{"status": "open"},
			"user":      map[string]any{"tier": "enterprise"},
			"order":     map[string]any{"total": 100},
			"recipient": map[string]any{"country": "NP"},
		},
		Variables: MapFacts{"minQuality": 0.93, "scoreFloor": 0.94},
		Candidates: []Candidate{
			{ID: "slow", Facts: MapFacts{"tiers": []string{"enterprise"}, "quality": 0.94, "cost": 0.020, "remaining_capacity": 200, "priority": 10, "specificity": 1, "fallback_order": 2, "blocked_countries": []string{"US"}}},
			{ID: "best", Facts: MapFacts{"tiers": []string{"enterprise"}, "quality": 0.98, "cost": 0.012, "remaining_capacity": 300, "priority": 10, "specificity": 2, "fallback_order": 1, "blocked_countries": []string{"US"}}},
			{ID: "blocked", Facts: MapFacts{"tiers": []string{"enterprise"}, "quality": 1.00, "cost": 0.010, "remaining_capacity": 300, "priority": 99, "specificity": 9, "fallback_order": 1, "blocked_countries": []string{"NP"}}},
		},
	}
	ranked, err := ranker.Rank(context.Background(), req)
	if err != nil {
		t.Fatalf("Rank returned error: %v", err)
	}
	if ranked.Winner == nil || ranked.Winner.ID != "best" {
		t.Fatalf("Rank winner = %#v, want best", ranked.Winner)
	}
	if ranked.Winner.Decision != "eligible" || len(ranked.Winner.Actions) != 1 {
		t.Fatalf("winner result lost action/decision: %#v", ranked.Winner)
	}
	selected, ok, err := ranker.SelectBestID(context.Background(), req)
	if err != nil || !ok {
		t.Fatalf("SelectBestID = %#v, %v, %v", selected, ok, err)
	}
	if selected.ID != ranked.Winner.ID || selected.Score != ranked.Winner.Score {
		t.Fatalf("SelectBestID = %#v, winner = %#v", selected, ranked.Winner)
	}
	var out RankingResult
	out.Candidates = make([]CandidateResult, 0, len(req.Candidates))
	if err := ranker.RankInto(context.Background(), req, &out); err != nil {
		t.Fatalf("RankInto returned error: %v", err)
	}
	if out.Winner == nil || out.Winner.ID != ranked.Winner.ID || out.Winner.Score != ranked.Winner.Score {
		t.Fatalf("RankInto winner = %#v, Rank winner = %#v", out.Winner, ranked.Winner)
	}
	var best CandidateResult
	ok, err = ranker.SelectBestInto(context.Background(), req, &best)
	if err != nil || !ok {
		t.Fatalf("SelectBestInto = %#v, %v, %v", best, ok, err)
	}
	if best.ID != ranked.Winner.ID || best.Score != ranked.Winner.Score {
		t.Fatalf("SelectBestInto = %#v, winner = %#v", best, ranked.Winner)
	}
}

func TestRankerNativeFastPathExplainReasons(t *testing.T) {
	ranker, err := NewRanker(RuleSet{
		Name: "reasons",
		Rules: []Rule{
			{ID: "active", Condition: `provider.active == true`, Reason: "provider inactive"},
		},
	})
	if err != nil {
		t.Fatalf("NewRanker returned error: %v", err)
	}
	res, err := ranker.Rank(context.Background(), RankingRequest{
		Candidates: []Candidate{{ID: "p1", Facts: MapFacts{"active": false}}},
	})
	if err != nil {
		t.Fatalf("Rank returned error: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].Eligible || len(res.Candidates[0].Reasons) != 1 || res.Candidates[0].Reasons[0] != "provider inactive" {
		t.Fatalf("candidate result = %#v", res.Candidates)
	}
}

func mustRouteTableRanker(t testing.TB) *Ranker {
	t.Helper()
	ranker, err := NewRanker(smsRouteRuleSet(), WithRankerTrace(true), WithRoutingTieBreakerPaths("route.priority", "route.specificity", "route.fallback_order", "quality.cost_per_sms"))
	if err != nil {
		t.Fatalf("NewRanker returned error: %v", err)
	}
	return ranker
}

func smsRouteRuleSet() RuleSet {
	return RuleSet{
		Name: "sms-route-table",
		Rules: []Rule{
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
		ScoreRules: []ScoreRule{
			{ID: "success-rate", Metric: "quality.success_rate", Weight: 0.60, Direction: HigherBetter, Normalize: Normalize{Min: 0.90, Max: 1}},
			{ID: "delivery-speed", Metric: "quality.avg_delivery_seconds", Weight: 0.20, Direction: LowerBetter, Normalize: Normalize{Min: 1, Max: 15}},
			{ID: "failure-rate", Metric: "quality.failure_rate", Weight: 0.10, Direction: LowerBetter, Normalize: Normalize{Min: 0, Max: 0.10}},
			{ID: "cost", Metric: "quality.cost_per_sms", Weight: 0.10, Direction: LowerBetter, Normalize: Normalize{Min: 0.003, Max: 0.03}},
		},
	}
}

func smsRouteRequest() RankingRequest {
	return RankingRequest{
		Facts: MapFacts{
			"user":      map[string]any{"id": 15, "tenant_id": 1001},
			"message":   map[string]any{"category": "otp", "type": "sms", "quality": "critical", "count": 500, "body": "Your OTP is 123456", "sender_id": "ACME"},
			"recipient": map[string]any{"number": "+9779800000000", "country": "NP", "carrier": "NCELL"},
		},
		Candidates: []Candidate{
			smsRouteCandidate("r-user-otp-np-ncell", "Sparrow SMS", smsRouteRow{UserID: 15, TenantID: 1001, Category: "otp", Type: "sms", Quality: "critical", Country: "NP", Carrier: "NCELL", MinCount: 1, MaxCount: 1000, Pattern: "OTP", SenderID: "ACME", Priority: 100, Fallback: 1}, smsProviderRow{ID: 1, Status: "active"}, smsQualityRow{SuccessRate: 0.991, AvgDeliverySeconds: 2, FailureRate: 0.004, CostPerSMS: 0.010}),
			smsRouteCandidate("r-user-otp-np", "Provider B", smsRouteRow{UserID: 15, TenantID: 1001, Category: "otp", Type: "sms", Quality: "critical", Country: "NP", MinCount: 1, MaxCount: 2000, Priority: 95, Fallback: 2}, smsProviderRow{ID: 2, Status: "active"}, smsQualityRow{SuccessRate: 0.985, AvgDeliverySeconds: 3, FailureRate: 0.006, CostPerSMS: 0.008}),
			smsRouteCandidate("r-user-default", "Provider C", smsRouteRow{UserID: 15, TenantID: 1001, Priority: 80, Fallback: 3}, smsProviderRow{ID: 3, Status: "active"}, smsQualityRow{SuccessRate: 0.970, AvgDeliverySeconds: 5, FailureRate: 0.012, CostPerSMS: 0.007}),
			smsRouteCandidate("r-global-otp-np", "Global OTP Provider", smsRouteRow{Category: "otp", Country: "NP", Priority: 70, Fallback: 4}, smsProviderRow{ID: 4, Status: "active"}, smsQualityRow{SuccessRate: 0.982, AvgDeliverySeconds: 4, FailureRate: 0.008, CostPerSMS: 0.006}),
			smsRouteCandidate("r-global-default", "Global Default Provider", smsRouteRow{Default: true, Priority: 10, Fallback: 99}, smsProviderRow{ID: 5, Status: "active"}, smsQualityRow{SuccessRate: 0.940, AvgDeliverySeconds: 9, FailureRate: 0.030, CostPerSMS: 0.004}),
			smsRouteCandidate("r-inactive-provider", "Inactive Provider", smsRouteRow{UserID: 15, Category: "otp", Country: "NP", Carrier: "NCELL", Priority: 999, Fallback: 1}, smsProviderRow{ID: 6, Status: "inactive"}, smsQualityRow{SuccessRate: 1, AvgDeliverySeconds: 1, FailureRate: 0, CostPerSMS: 0.001}),
			smsRouteCandidate("r-too-small", "Small Batch Provider", smsRouteRow{UserID: 15, Category: "otp", Country: "NP", Carrier: "NCELL", MaxCount: 100, Priority: 1000, Fallback: 1}, smsProviderRow{ID: 7, Status: "active"}, smsQualityRow{SuccessRate: 1, AvgDeliverySeconds: 1, FailureRate: 0, CostPerSMS: 0.001}),
		},
	}
}

type smsRouteRow struct {
	UserID, TenantID                          any
	Category, Type, Quality, Country, Carrier string
	MinCount, MaxCount                        any
	Pattern, SenderID                         string
	Priority, Fallback                        int
	Weight                                    int
	Default                                   bool
}

type smsProviderRow struct {
	ID     int
	Status string
}

type smsQualityRow struct {
	SuccessRate, AvgDeliverySeconds, FailureRate, CostPerSMS float64
}

func smsRouteCandidate(id, providerName string, route smsRouteRow, provider smsProviderRow, quality smsQualityRow) Candidate {
	routeMap := MapFacts{
		"id": id, "user_id": nullableTest(route.UserID), "tenant_id": nullableTest(route.TenantID),
		"message_category": nullableStringTest(route.Category), "message_type": nullableStringTest(route.Type),
		"delivery_quality": nullableStringTest(route.Quality), "recipient_country": nullableStringTest(route.Country),
		"carrier_code": nullableStringTest(route.Carrier), "min_message_count": nullableTest(route.MinCount),
		"max_message_count": nullableTest(route.MaxCount), "message_pattern": nullableStringTest(route.Pattern),
		"sender_id": nullableStringTest(route.SenderID), "priority": route.Priority, "fallback_order": route.Fallback,
		"weight": route.Weight, "is_default": route.Default, "status": "active", "valid_from": nil, "valid_to": nil,
	}
	routeMap["specificity"] = smsRouteSpecificity(routeMap)
	return Candidate{ID: id, Name: providerName, Facts: MapFacts{
		"route":    routeMap,
		"provider": MapFacts{"id": provider.ID, "name": providerName, "status": provider.Status},
		"quality":  MapFacts{"success_rate": quality.SuccessRate, "avg_delivery_seconds": quality.AvgDeliverySeconds, "failure_rate": quality.FailureRate, "cost_per_sms": quality.CostPerSMS},
	}}
}

func smsRouteSpecificity(route MapFacts) int {
	score := 0
	weights := map[string]int{"user_id": 10, "message_category": 8, "message_type": 6, "delivery_quality": 6, "recipient_country": 5, "carrier_code": 5, "min_message_count": 3, "max_message_count": 3, "message_pattern": 4}
	for key, weight := range weights {
		if route[key] != nil {
			score += weight
		}
	}
	return score
}

func nullableTest(v any) any {
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

func nullableStringTest(v string) any {
	if v == "" {
		return nil
	}
	return v
}
