package condition

import (
	"context"
	"testing"
)

var benchFacts = MapFacts{
	"user":  map[string]any{"age": 30, "tier": "vip", "email": "a@example.com"},
	"order": map[string]any{"total": 125.50},
	"tags":  []string{"vip", "beta", "active"},
}

func BenchmarkCompile(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := Compile(`user.age >= 18 and order.total > 100 and "vip" in tags`); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvalMapFacts(b *testing.B) {
	expr := MustCompile(`user.age >= 18 and order.total > 100 and "vip" in tags`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.Eval(context.Background(), benchFacts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvalBoolMapFacts(b *testing.B) {
	expr := MustCompile(`user.age >= 18 and order.total > 100 and "vip" in tags`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.EvalBool(context.Background(), benchFacts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvalWithVariables(b *testing.B) {
	expr := MustCompile(`minAge <= user.age and minTotal < order.total and "vip" in tags`, WithVariables(MapFacts{
		"minAge":   18,
		"minTotal": 100,
	}))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.EvalBool(context.Background(), benchFacts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvalTypedGetterFacts(b *testing.B) {
	expr := MustCompile(`user.age >= 18 and order.total > 100 and tag == "vip"`)
	facts := FactFunc(func(path string) (any, bool) {
		switch path {
		case "user.age":
			return 30, true
		case "order.total":
			return 125.50, true
		case "tag":
			return "vip", true
		default:
			return nil, false
		}
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.EvalBool(context.Background(), facts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvalStructFacts(b *testing.B) {
	type User struct {
		Age  int    `json:"age"`
		Tier string `json:"tier"`
	}
	type Order struct {
		Total float64 `json:"total"`
	}
	type FactsStruct struct {
		User  User     `json:"user"`
		Order Order    `json:"order"`
		Tags  []string `json:"tags"`
	}
	expr := MustCompile(`user.age >= 18 and order.total > 100 and "vip" in tags`)
	facts := NewStructFacts(FactsStruct{
		User:  User{Age: 30, Tier: "vip"},
		Order: Order{Total: 125.50},
		Tags:  []string{"vip", "beta", "active"},
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.Eval(context.Background(), facts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAggregationEval(b *testing.B) {
	facts := MapFacts{
		"items": []any{
			map[string]any{"amount": 120, "risk": 0.10, "active": true},
			map[string]any{"amount": 80, "risk": 0.20, "active": true},
			map[string]any{"amount": 40, "risk": 0.05, "active": false},
		},
	}
	expr := MustCompile(`sum(items, "amount") >= 200 and avg(items, "risk") < 0.2 and any(items, "active")`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.EvalBool(context.Background(), facts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConditionalDefaultFunctions(b *testing.B) {
	expr := MustCompile(`(user.age >= 18 ? coalesce(user.nickname, user.tier, "guest") : "minor") == "vip" and defaultValue(order.discount, 0) == 0`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.EvalBool(context.Background(), benchFacts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDateTimeHelpers(b *testing.B) {
	expr := MustCompile(`before(order.created_at, now()) and betweenTime(order.created_at, "2026-01-01T00:00:00Z", "2026-12-31T23:59:59Z") and age(order.created_at, "hour") >= 0`)
	facts := MapFacts{"order": map[string]any{"created_at": "2026-05-01T10:30:00Z"}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.EvalBool(context.Background(), facts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCollectionHelpers(b *testing.B) {
	facts := MapFacts{"items": benchRows()}
	expr := MustCompile(`count(filter(items, "status", "==", "paid")) == 3 and get(top(items, "amount"), "amount") >= 150 and percentile(items, "amount", 90) >= 100`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.EvalBool(context.Background(), facts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGroupedAggregateWhere(b *testing.B) {
	facts := MapFacts{"items": benchRows()}
	expr := MustCompile(`groupCountWhere(items, "status", "paid", "amount", ">", 100) == 2 and groupAvgWhere(items, "status", "paid", "risk", "<", 0.2, "risk") < 0.2`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := expr.EvalBool(context.Background(), facts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRouteTableSelectBestID(b *testing.B) {
	ranker := mustRouteTableRanker(b)
	req := smsRouteRequest()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := ranker.SelectBestID(context.Background(), req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRouteTableWeightedSelection(b *testing.B) {
	ranker, err := NewRanker(
		RuleSet{Name: "weighted", Rules: []Rule{{ID: "active", Condition: `active(route.status)`}}},
		WithRoutingTieBreakerPaths("route.priority", "route.specificity", "route.fallback_order", "quality.cost_per_sms"),
		WithWeightedSelection("route.weight", "user.id"),
	)
	if err != nil {
		b.Fatal(err)
	}
	req := RankingRequest{
		Facts: MapFacts{"user": map[string]any{"id": "u-1"}},
		Candidates: []Candidate{
			{ID: "a", Facts: MapFacts{"route": MapFacts{"status": "active", "priority": 10, "specificity": 1, "fallback_order": 1, "weight": 70}}},
			{ID: "b", Facts: MapFacts{"route": MapFacts{"status": "active", "priority": 10, "specificity": 1, "fallback_order": 2, "weight": 30}}},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := ranker.SelectWeightedID(context.Background(), req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRouteTableFallbacks(b *testing.B) {
	ranker := mustRouteTableRanker(b)
	req := smsRouteRequest()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ranker.SelectFallbacks(context.Background(), req, 3); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLargeRuleSet(b *testing.B) {
	engine := NewEngine()
	rules := make([]Rule, 100)
	for i := range rules {
		rules[i] = Rule{ID: string(rune('a'+i%26)) + string(rune('a'+i/26)), Condition: `user.age >= 18 and order.total > 100`, Score: 1}
	}
	if err := engine.AddRuleSet(RuleSet{Name: "large", Rules: rules}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.Evaluate(context.Background(), benchFacts, "large"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRanker5Candidates(b *testing.B) {
	benchmarkRanker(b, 5, false)
}

func BenchmarkRanker25Candidates(b *testing.B) {
	benchmarkRanker(b, 25, false)
}

func BenchmarkRanker100Candidates(b *testing.B) {
	benchmarkRanker(b, 100, false)
}

func BenchmarkRankerWithVariables(b *testing.B) {
	benchmarkRanker(b, 25, true)
}

func BenchmarkRankerIntoNoExplain25Candidates(b *testing.B) {
	rules := RuleSet{
		Name: "bench-routing-into",
		Rules: []Rule{
			{ID: "tier", Condition: `user.tier in provider.user_tiers`},
			{ID: "type", Condition: `message.type in provider.message_types`},
			{ID: "country", Condition: `recipient.country in provider.countries`},
			{ID: "carrier", Condition: `recipient.carrier in provider.carriers or "*" in provider.carriers`},
			{ID: "capacity", Condition: `message.count <= provider.remaining_capacity`},
		},
		ScoreRules: []ScoreRule{
			{ID: "quality", Metric: "provider.quality", Weight: 0.55, Direction: HigherBetter, Normalize: Normalize{Min: 0.90, Max: 1}},
			{ID: "cost", Metric: "provider.cost", Weight: 0.30, Direction: LowerBetter, Normalize: Normalize{Min: 0.005, Max: 0.03}},
			{ID: "priority", Metric: "provider.priority", Weight: 0.15, Direction: HigherBetter, Normalize: Normalize{Min: 0, Max: 10}},
		},
	}
	ranker, err := NewRanker(rules, WithRankerExplain(false))
	if err != nil {
		b.Fatal(err)
	}
	req := RankingRequest{
		Facts: MapFacts{
			"user":      map[string]any{"tier": "enterprise"},
			"message":   map[string]any{"count": 500, "type": "transactional"},
			"recipient": map[string]any{"country": "US", "carrier": "Verizon"},
		},
		Candidates: makeBenchCandidates(25),
	}
	var out RankingResult
	out.Candidates = make([]CandidateResult, 0, len(req.Candidates))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ranker.RankInto(context.Background(), req, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRankerSelectBestNoExplain25Candidates(b *testing.B) {
	rules := RuleSet{
		Name: "bench-routing-select",
		Rules: []Rule{
			{ID: "tier", Condition: `user.tier in provider.user_tiers`},
			{ID: "type", Condition: `message.type in provider.message_types`},
			{ID: "country", Condition: `recipient.country in provider.countries`},
			{ID: "carrier", Condition: `recipient.carrier in provider.carriers or "*" in provider.carriers`},
			{ID: "capacity", Condition: `message.count <= provider.remaining_capacity`},
		},
		ScoreRules: []ScoreRule{
			{ID: "quality", Metric: "provider.quality", Weight: 0.55, Direction: HigherBetter, Normalize: Normalize{Min: 0.90, Max: 1}},
			{ID: "cost", Metric: "provider.cost", Weight: 0.30, Direction: LowerBetter, Normalize: Normalize{Min: 0.005, Max: 0.03}},
			{ID: "priority", Metric: "provider.priority", Weight: 0.15, Direction: HigherBetter, Normalize: Normalize{Min: 0, Max: 10}},
		},
	}
	ranker, err := NewRanker(rules, WithRankerExplain(false))
	if err != nil {
		b.Fatal(err)
	}
	req := RankingRequest{
		Facts: MapFacts{
			"user":      map[string]any{"tier": "enterprise"},
			"message":   map[string]any{"count": 500, "type": "transactional"},
			"recipient": map[string]any{"country": "US", "carrier": "Verizon"},
		},
		Candidates: makeBenchCandidates(25),
	}
	var best CandidateResult
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ranker.SelectBestInto(context.Background(), req, &best); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRankerSelectBestID25Candidates(b *testing.B) {
	rules := RuleSet{
		Name: "bench-routing-select-id",
		Rules: []Rule{
			{ID: "tier", Condition: `user.tier in provider.user_tiers`},
			{ID: "type", Condition: `message.type in provider.message_types`},
			{ID: "country", Condition: `recipient.country in provider.countries`},
			{ID: "carrier", Condition: `recipient.carrier in provider.carriers or "*" in provider.carriers`},
			{ID: "capacity", Condition: `message.count <= provider.remaining_capacity`},
		},
		ScoreRules: []ScoreRule{
			{ID: "quality", Metric: "provider.quality", Weight: 0.55, Direction: HigherBetter, Normalize: Normalize{Min: 0.90, Max: 1}},
			{ID: "cost", Metric: "provider.cost", Weight: 0.30, Direction: LowerBetter, Normalize: Normalize{Min: 0.005, Max: 0.03}},
			{ID: "priority", Metric: "provider.priority", Weight: 0.15, Direction: HigherBetter, Normalize: Normalize{Min: 0, Max: 10}},
		},
	}
	ranker, err := NewRanker(rules, WithRankerExplain(false))
	if err != nil {
		b.Fatal(err)
	}
	req := RankingRequest{
		Facts: MapFacts{
			"user":      map[string]any{"tier": "enterprise"},
			"message":   map[string]any{"count": 500, "type": "transactional"},
			"recipient": map[string]any{"country": "US", "carrier": "Verizon"},
		},
		Candidates: makeBenchCandidates(25),
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := ranker.SelectBestID(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkRanker(b *testing.B, n int, withVars bool) {
	rules := RuleSet{
		Name: "bench-routing",
		Rules: []Rule{
			{ID: "tier", Condition: `user.tier in provider.user_tiers`},
			{ID: "type", Condition: `message.type in provider.message_types`},
			{ID: "country", Condition: `recipient.country in provider.countries`},
			{ID: "capacity", Condition: `message.count <= provider.remaining_capacity`},
		},
		ScoreRules: []ScoreRule{
			{ID: "quality", Metric: "provider.quality", Weight: 0.55, Direction: HigherBetter, Normalize: Normalize{Min: 0.90, Max: 1}},
			{ID: "cost", Metric: "provider.cost", Weight: 0.30, Direction: LowerBetter, Normalize: Normalize{Min: 0.005, Max: 0.03}},
			{ID: "priority", Metric: "provider.priority", Weight: 0.15, Direction: HigherBetter, Normalize: Normalize{Min: 0, Max: 10}},
		},
	}
	if withVars {
		rules.Rules = append(rules.Rules, Rule{ID: "min-quality", Condition: `provider.quality >= $minQuality`})
	}
	ranker, err := NewRanker(rules)
	if err != nil {
		b.Fatal(err)
	}
	req := RankingRequest{
		Facts: MapFacts{
			"user":      map[string]any{"tier": "enterprise"},
			"message":   map[string]any{"count": 500, "type": "transactional"},
			"recipient": map[string]any{"country": "US"},
		},
		Candidates: makeBenchCandidates(n),
	}
	if withVars {
		req.Variables = MapFacts{"minQuality": 0.91}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ranker.Rank(context.Background(), req); err != nil {
			b.Fatal(err)
		}
	}
}

func benchRows() []any {
	return []any{
		map[string]any{"status": "paid", "amount": 150, "risk": 0.10},
		map[string]any{"status": "paid", "amount": 75, "risk": 0.05},
		map[string]any{"status": "failed", "amount": 20, "risk": 0.50},
		map[string]any{"status": "paid", "amount": 125, "risk": 0.20},
		map[string]any{"status": "open", "amount": 40, "risk": 0.15},
	}
}

func makeBenchCandidates(n int) []Candidate {
	candidates := make([]Candidate, n)
	for i := range candidates {
		candidates[i] = Candidate{
			ID: "provider-" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Facts: MapFacts{
				"user_tiers":         []string{"enterprise", "startup"},
				"message_types":      []string{"transactional", "marketing"},
				"countries":          []string{"US", "CA", "NP"},
				"carriers":           []string{"Verizon", "NCell", "*"},
				"quality":            0.92 + float64(i%8)/100,
				"cost":               0.006 + float64(i%5)/1000,
				"remaining_capacity": 1000 + i,
				"priority":           i % 10,
			},
		}
	}
	return candidates
}
