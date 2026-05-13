package condition

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestExpressionOperatorsAndPrecedence(t *testing.T) {
	facts := MapFacts{
		"user": map[string]any{"age": 21, "name": "Sujit", "email": "sujit@example.com"},
		"tags": []string{"vip", "beta"},
		"plan": "pro",
	}
	tests := []struct {
		expr string
		want bool
	}{
		{`user.age >= 18 and plan == "pro"`, true},
		{`user.age < 18 or plan == "pro"`, true},
		{`user.age < 18 and plan == "pro"`, false},
		{`"vip" in tags`, true},
		{`"free" not in tags`, true},
		{`"@" in user.email`, true},
		{`regex_match(user.email, "^[^@]+@example\\.com$")`, true},
		{`starts_with(user.name, "Su")`, true},
		{`ends_with(user.name, "it")`, true},
		{`not (user.age < 18)`, true},
		{`false and missing.path == 1`, false},
		{`true or missing.path == 1`, true},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			res, err := Eval(tt.expr, facts)
			if err != nil {
				t.Fatalf("Eval returned error: %v", err)
			}
			if res.Matched != tt.want {
				t.Fatalf("Matched = %v, want %v", res.Matched, tt.want)
			}
		})
	}
}

func TestFunctionsAndTrace(t *testing.T) {
	expr := MustCompile(`exists("user.age") and len(tags) == 2 and not empty(user.name)`, WithTrace(true))
	res, err := expr.Eval(context.Background(), MapFacts{
		"user": map[string]any{"age": 30, "name": "Asha"},
		"tags": []any{"a", "b"},
	})
	if err != nil {
		t.Fatalf("Eval returned error: %v", err)
	}
	if !res.Matched {
		t.Fatal("expected expression to match")
	}
	if !res.Trace.Enabled || len(res.Trace.Steps) == 0 || res.Trace.Duration == "" {
		t.Fatalf("expected populated trace, got %#v", res.Trace)
	}
}

func TestAggregationFunctions(t *testing.T) {
	facts := MapFacts{
		"values": []int{2, 4, 6},
		"orders": []any{
			map[string]any{"amount": 120, "status": "paid", "risk": 0.10, "active": true},
			map[string]any{"amount": 80, "status": "pending", "risk": 0.30, "active": true},
			map[string]any{"amount": 40, "status": "paid", "risk": 0.05, "active": false},
		},
	}
	tests := []struct {
		expr string
		want bool
	}{
		{`sum(values) == 12`, true},
		{`avg(values) == 4`, true},
		{`min(values) == 2 and max(values) == 6`, true},
		{`count(orders) == 3`, true},
		{`sum(orders, "amount") == 240`, true},
		{`avg(orders, "risk") < 0.2`, true},
		{`countWhere(orders, "status", "==", "paid") == 2`, true},
		{`any(orders, "amount", ">=", 100)`, true},
		{`all(orders, "amount", ">", 10)`, true},
		{`none(orders, "risk", ">", 0.5)`, true},
		{`any(orders, "active")`, true},
		{`"paid" in groupBy(orders, "status")`, true},
		{`groupCount(orders, "status", "paid") == 2`, true},
		{`groupSum(orders, "status", "paid", "amount") == 160`, true},
		{`groupAvg(orders, "status", "paid", "risk") < 0.1`, true},
		{`groupMin(orders, "status", "paid", "amount") == 40`, true},
		{`groupMax(orders, "status", "paid", "amount") == 120`, true},
		{`distinctCount(orders, "status") == 2`, true},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			matched, err := MustCompile(tt.expr).EvalBool(context.Background(), facts)
			if err != nil {
				t.Fatalf("EvalBool returned error: %v", err)
			}
			if matched != tt.want {
				t.Fatalf("matched = %v, want %v", matched, tt.want)
			}
		})
	}
}

func TestAdvancedDSLHelpers(t *testing.T) {
	now := time.Now().UTC()
	facts := MapFacts{
		"name":        "  Sujit  ",
		"empty_value": "",
		"score":       -2.6,
		"created_at":  now.Add(-48 * time.Hour).Format(time.RFC3339),
		"valid_from":  now.Add(-time.Hour).Format(time.RFC3339),
		"valid_to":    now.Add(time.Hour).Format(time.RFC3339),
		"route": map[string]any{
			"user_id": nil,
			"min":     nil,
			"max":     100,
			"status":  "active",
		},
		"user": map[string]any{"id": 42},
	}
	tests := []string{
		`(score < 0 ? "low" : "high") == "low"`,
		`coalesce(empty_value, null, "fallback") == "fallback"`,
		`defaultValue(empty_value, "fallback") == "fallback"`,
		`isNull(route.user_id) and isNotNull(user.id)`,
		`lower(trim(name)) == "sujit" and upper("np") == "NP"`,
		`hasPrefix(trim(name), "Su") and hasSuffix(trim(name), "it")`,
		`join(split("a,b,c", ","), "|") == "a|b|c"`,
		`abs(score) > 2 and round(score) == -3 and floor(2.9) == 2 and ceil(2.1) == 3`,
		`clamp(120, 0, 100) == 100 and between(50, 1, 100)`,
		`before(created_at, now()) and after(now(), created_at)`,
		`betweenTime(now(), valid_from, valid_to)`,
		`age(created_at, "hours") >= 47`,
		`nullableMatch(route.user_id, user.id) and rangeMatch(route.min, route.max, 50)`,
		`active(route.status) and validNow(valid_from, valid_to)`,
		`specificity(route.user_id, user.id, route.max) == 2`,
		`stableBucket(user.id, 100) >= 0`,
	}
	for _, expr := range tests {
		t.Run(expr, func(t *testing.T) {
			matched, err := MustCompile(expr).EvalBool(context.Background(), facts)
			if err != nil {
				t.Fatalf("EvalBool returned error: %v", err)
			}
			if !matched {
				t.Fatal("expected expression to match")
			}
		})
	}
}

func TestCollectionEnhancements(t *testing.T) {
	facts := MapFacts{
		"orders": []any{
			map[string]any{"amount": 120, "status": "paid", "risk": 0.10},
			map[string]any{"amount": 80, "status": "pending", "risk": 0.30},
			map[string]any{"amount": 40, "status": "paid", "risk": 0.05},
		},
	}
	tests := []string{
		`count(filter(orders, "status", "==", "paid")) == 2`,
		`sum(filter(orders, "status", "==", "paid"), "amount") == 160`,
		`count(pluck(orders, "amount")) == 3`,
		`get(first(orders), "amount") == 120`,
		`get(last(orders), "amount") == 40`,
		`get(top(orders, "amount"), "amount") == 120`,
		`get(bottom(orders, "amount"), "amount") == 40`,
		`percentile(orders, "amount", 50) == 80`,
		`get(first(sortBy(orders, "amount")), "amount") == 40`,
		`get(first(sortBy(orders, "amount", "desc")), "amount") == 120`,
		`sum(take(sortBy(orders, "amount", "desc"), 2), "amount") == 200`,
		`count(skip(orders, 2)) == 1`,
		`count(slice(orders, 1, 3)) == 2`,
		`get(first(reverse(orders)), "amount") == 40`,
		`join(distinct(orders, "status"), "|") == "paid|pending"`,
		`groupCountWhere(orders, "status", "paid", "amount", ">", 50) == 1`,
		`groupSumWhere(orders, "status", "paid", "amount", ">", 30, "amount") == 160`,
		`groupAvgWhere(orders, "status", "paid", "amount", ">", 30, "risk") < 0.1`,
	}
	for _, expr := range tests {
		t.Run(expr, func(t *testing.T) {
			matched, err := MustCompile(expr).EvalBool(context.Background(), facts)
			if err != nil {
				t.Fatalf("EvalBool returned error: %v", err)
			}
			if !matched {
				t.Fatal("expected expression to match")
			}
		})
	}
}

func TestVariablesAndPlaceholders(t *testing.T) {
	facts := MapFacts{
		"user": map[string]any{"age": 29, "country": "NP"},
		"tags": []string{"vip", "trusted"},
	}
	vars := MapFacts{
		"minAge":      18,
		"country":     map[string]any{"allowed": "NP"},
		"allowedTags": []string{"vip", "staff"},
	}
	tests := []string{
		`user.age >= minAge and user.country == country.allowed`,
		`user.age >= minAge and "vip" in allowedTags`,
		`minAge <= user.age and country.allowed == user.country`,
		`minAge <= user.age and "vip" in allowedTags`,
		`user.age >= $minAge and user.country == $country.allowed`,
		`$minAge <= user.age and "vip" in $allowedTags`,
		`user.age >= {{ minAge }} and user.country == {{ country.allowed }}`,
		`{{minAge}} <= user.age and "vip" in {{allowedTags}}`,
		`user.age >= ${minAge} and user.country == ${country.allowed}`,
	}
	for _, expr := range tests {
		t.Run(expr, func(t *testing.T) {
			compiled, err := Compile(expr, WithVariables(vars))
			if err != nil {
				t.Fatalf("Compile returned error: %v", err)
			}
			res, err := compiled.Eval(context.Background(), facts)
			if err != nil {
				t.Fatalf("Eval returned error: %v", err)
			}
			if !res.Matched {
				t.Fatal("expected placeholder expression to match")
			}
		})
	}

	compiled := MustCompile(`user.age >= minAge`)
	res, err := compiled.EvalWithVariables(context.Background(), facts, MapFacts{"minAge": 30})
	if err != nil {
		t.Fatalf("EvalWithVariables returned error: %v", err)
	}
	if res.Matched {
		t.Fatal("expected per-evaluation variables to override as non-match")
	}
}

func TestCompileTimeInterpolationAcrossEntryPoints(t *testing.T) {
	interpolation := MapFacts{
		"minAge":    18,
		"country":   "NP",
		"riskLimit": 0.75,
		"tags":      []string{"vip", "staff"},
	}
	expr := MustCompile(
		`user.age >= {{minAge}} and user.country == {{country}} and risk < {{riskLimit}} and "vip" in {{tags}}`,
		WithInterpolation(interpolation),
	)
	matched, err := expr.EvalBool(context.Background(), MapFacts{
		"user": map[string]any{"age": 29, "country": "NP"},
		"risk": 0.4,
	})
	if err != nil {
		t.Fatalf("EvalBool returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected interpolated expression to match")
	}

	engine := NewEngine(WithExpressionOptions(WithInterpolationMap(map[string]any{
		"minAge":  18,
		"country": "NP",
	})))
	if err := engine.AddRuleSet(RuleSet{Name: "interpolated", Rules: []Rule{{
		ID:        "1",
		Condition: `user.age >= {{minAge}} and user.country == {{country}}`,
		Decision:  "allow",
	}}}); err != nil {
		t.Fatalf("AddRuleSet returned error: %v", err)
	}
	res, err := engine.Evaluate(context.Background(), MapFacts{"user": map[string]any{"age": 29, "country": "NP"}}, "interpolated")
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !res.Matched || !res.Allowed {
		t.Fatalf("expected interpolated engine rule to allow, got %#v", res)
	}

	ranker, err := NewRanker(
		RuleSet{Name: "rank-interpolated", Rules: []Rule{{ID: "eligible", Condition: `quality >= {{minQuality}}`}}},
		WithRankerExpressionOptions(WithInterpolationMap(map[string]any{"minQuality": 0.9})),
	)
	if err != nil {
		t.Fatalf("NewRanker returned error: %v", err)
	}
	ranked, err := ranker.Rank(context.Background(), RankingRequest{
		Candidates: []Candidate{{ID: "p1", Facts: MapFacts{"quality": 0.94}}},
	})
	if err != nil {
		t.Fatalf("Rank returned error: %v", err)
	}
	if len(ranked.Candidates) != 1 || !ranked.Candidates[0].Eligible {
		t.Fatalf("expected interpolated ranker candidate eligible, got %#v", ranked.Candidates)
	}
}

func TestInterpolationNotRewrittenInStringsAndMissingErrors(t *testing.T) {
	for _, expr := range []string{`"{{minAge}}" == "{{minAge}}"`, `"${minAge}" == "${minAge}"`} {
		normalized, err := normalizeExpressionVariables(expr, MapFacts{"minAge": 18})
		if err != nil {
			t.Fatalf("normalizeExpressionVariables returned error: %v", err)
		}
		if normalized != expr {
			t.Fatalf("normalized expression = %q, want %q", normalized, expr)
		}
	}

	_, err := Compile(`user.age >= {{missing}}`, WithInterpolationMap(map[string]any{}))
	if err == nil {
		t.Fatal("expected missing interpolation variable error")
	}
	condErr, ok := err.(*Error)
	if !ok || condErr.Kind != ErrMissing {
		t.Fatalf("error = %#v, want ErrMissing", err)
	}
}

func TestDollarVariablesAcrossEntryPoints(t *testing.T) {
	facts := MapFacts{"provider": map[string]any{"quality": 0.94}}
	vars := MapFacts{"minQuality": 0.91}
	expr := MustCompile(`provider.quality >= $minQuality`, WithVariables(vars))
	matched, err := expr.EvalBool(context.Background(), facts)
	if err != nil {
		t.Fatalf("EvalBool returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected dollar variable expression to match")
	}
	res, err := expr.EvalWithVariables(context.Background(), facts, vars)
	if err != nil {
		t.Fatalf("EvalWithVariables returned error: %v", err)
	}
	if !res.Matched {
		t.Fatal("expected per-eval dollar variable expression to match")
	}

	engine := NewEngine(WithExpressionOptions(WithVariables(vars)))
	if err := engine.AddRuleSet(RuleSet{Name: "vars", Rules: []Rule{{ID: "quality", Condition: `provider.quality >= $minQuality`}}}); err != nil {
		t.Fatalf("AddRuleSet returned error: %v", err)
	}
	er, err := engine.Evaluate(context.Background(), facts, "vars")
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !er.Matched {
		t.Fatal("expected engine dollar variable rule to match")
	}

	ranker, err := NewRanker(RuleSet{Name: "rank-vars", Rules: []Rule{{ID: "quality", Condition: `provider.quality >= $minQuality`}}})
	if err != nil {
		t.Fatalf("NewRanker returned error: %v", err)
	}
	rr, err := ranker.Rank(context.Background(), RankingRequest{
		Variables:  vars,
		Candidates: []Candidate{{ID: "p1", Facts: MapFacts{"quality": 0.94}}},
	})
	if err != nil {
		t.Fatalf("Rank returned error: %v", err)
	}
	if len(rr.Candidates) != 1 || !rr.Candidates[0].Eligible {
		t.Fatalf("expected ranker candidate eligible, got %#v", rr.Candidates)
	}
}

func TestDollarVariablesNotRewrittenInStrings(t *testing.T) {
	res, err := Eval(`"$minQuality" == "$minQuality"`, MapFacts{})
	if err != nil {
		t.Fatalf("Eval returned error: %v", err)
	}
	if !res.Matched {
		t.Fatal("expected literal dollar string to remain unchanged")
	}
}

func TestNativeHelperExpressionsMatchInterpreterFallback(t *testing.T) {
	facts := MapFacts{
		"user":  map[string]any{"age": 30, "tier": "vip"},
		"order": map[string]any{"created_at": "2026-05-01T10:30:00Z", "discount": 0},
		"items": benchRows(),
	}
	expressions := []string{
		`sum(items, "amount") >= 200 and avg(items, "risk") < 0.2 and any(items, "status", "==", "paid")`,
		`count(filter(items, "status", "==", "paid")) == 3 and get(top(items, "amount"), "amount") >= 150 and percentile(items, "amount", 90) >= 100`,
		`groupCountWhere(items, "status", "paid", "amount", ">", 100) == 2 and groupAvgWhere(items, "status", "paid", "risk", "<", 0.2, "risk") < 0.2`,
		`(user.age >= 18 ? coalesce(user.nickname, user.tier, "guest") : "minor") == "vip" and defaultValue(order.discount, 0) == 0`,
		`before(order.created_at, now()) and betweenTime(order.created_at, "2026-01-01T00:00:00Z", "2026-12-31T23:59:59Z") and age(order.created_at, "hour") >= 0`,
	}
	custom := NewFunctionRegistry()
	registerDSLHelperFunctions(custom)
	registerAggregateFunctions(custom)
	for _, expr := range expressions {
		t.Run(expr, func(t *testing.T) {
			native := MustCompile(expr)
			nativeResult, err := native.EvalBool(context.Background(), facts)
			if err != nil {
				t.Fatalf("native EvalBool returned error: %v", err)
			}
			fallback := MustCompile(expr, WithFunctions(custom))
			fallbackResult, err := fallback.EvalBool(context.Background(), facts)
			if err != nil {
				t.Fatalf("fallback EvalBool returned error: %v", err)
			}
			if nativeResult != fallbackResult {
				t.Fatalf("native result = %v, fallback result = %v", nativeResult, fallbackResult)
			}
		})
	}
}

func TestCustomFunctionsUseInterpreterBridge(t *testing.T) {
	funcs := NewFunctionRegistry()
	funcs.Register("isVIP", func(_ EvalContext, args ...any) (any, error) {
		return len(args) == 1 && args[0] == "vip", nil
	})
	expr := MustCompile(`isVIP(user.tier)`, WithFunctions(funcs))
	matched, err := expr.EvalBool(context.Background(), MapFacts{"user": map[string]any{"tier": "vip"}})
	if err != nil {
		t.Fatalf("EvalBool returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected custom bridged function to match")
	}
}

func TestOldCustomSyntaxRejected(t *testing.T) {
	if _, err := Compile(`user.email matches "example"`); err == nil {
		t.Fatal("expected old matches operator syntax to fail")
	}
}

func TestStrictMissingFact(t *testing.T) {
	expr := MustCompile(`missing.value == 1`, Strict(true))
	_, err := expr.Eval(context.Background(), MapFacts{})
	if err == nil {
		t.Fatal("expected strict missing fact error")
	}
	if got := err.(*Error).Kind; got != ErrMissing {
		t.Fatalf("error kind = %s, want %s", got, ErrMissing)
	}
}

func TestFactProviders(t *testing.T) {
	type Profile struct {
		Age  int    `json:"age"`
		Name string `json:"name"`
	}
	type User struct {
		Profile Profile `json:"profile"`
	}
	structFacts := NewStructFacts(User{Profile: Profile{Age: 42, Name: "Mira"}})
	fnFacts := FactFunc(func(path string) (any, bool) {
		if path == "country" {
			return "NP", true
		}
		return nil, false
	})
	res, err := Eval(`profile.age >= 40 and country == "NP"`, Chain(structFacts, fnFacts))
	if err != nil {
		t.Fatalf("Eval returned error: %v", err)
	}
	if !res.Matched {
		t.Fatal("expected chained facts to match")
	}
}

func TestBuilderCompilesToSameDSL(t *testing.T) {
	b := When("user.age").Gte(18).And("country").In("US", "CA")
	if b.String() != `(user.age >= 18) and country in ["US", "CA"]` {
		t.Fatalf("builder string = %q", b.String())
	}
	expr, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}
	res, err := expr.Eval(context.Background(), MapFacts{
		"user":    map[string]any{"age": 19},
		"country": "CA",
	})
	if err != nil {
		t.Fatalf("Eval returned error: %v", err)
	}
	if !res.Matched {
		t.Fatal("expected builder expression to match")
	}
}

func TestBuilderVariableOnLeft(t *testing.T) {
	b := Var("minAge").LtePath("user.age").And("required.country").EqPath("user.country")
	if b.String() != `(minAge <= user.age) and required.country == user.country` {
		t.Fatalf("builder string = %q", b.String())
	}
	expr, err := b.Compile(WithVariables(MapFacts{
		"minAge": 18,
		"required": map[string]any{
			"country": "NP",
		},
	}))
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}
	matched, err := expr.EvalBool(context.Background(), MapFacts{
		"user": map[string]any{"age": 29, "country": "NP"},
	})
	if err != nil {
		t.Fatalf("EvalBool returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected builder variable expression to match")
	}
}

func TestBuilderErgonomics(t *testing.T) {
	checks := []struct {
		name string
		b    Builder
		want string
	}{
		{"expr", Expr(` user.active `), `user.active`},
		{"exists", When("user.email").Exists(), `exists("user.email")`},
		{"missing", When("user.deleted_at").Missing(), `not exists("user.deleted_at")`},
		{"null", When("user.nickname").IsNull(), `isNull(user.nickname)`},
		{"not-null", When("user.name").IsNotNull(), `isNotNull(user.name)`},
		{"empty", When("user.nickname").Empty(), `empty(user.nickname)`},
		{"not-empty", When("user.name").NotEmpty(), `not empty(user.name)`},
		{"between", When("score").Between(10, 20), `between(score, 10, 20)`},
		{"between-path", When("score").BetweenPath("limits.min", "limits.max"), `between(score, limits.min, limits.max)`},
		{"in-path", When("user.country").InPath("allowed.countries"), `user.country in allowed.countries`},
		{"not-in-path", When("user.country").NotInPath("blocked.countries"), `user.country not in blocked.countries`},
		{"and-var", When("user.age").Gte(18).AndVar("$maxAge").GtePath("user.age"), `(user.age >= 18) and maxAge >= user.age`},
		{"or-var", When("user.age").Lt(18).OrVar("override").Eq(true), `(user.age < 18) or override == true`},
		{"and-expr", When("user.age").Gte(18).AndExpr(When("plan").Eq("pro")), `(user.age >= 18) and (plan == "pro")`},
		{"or-expr", When("plan").Eq("pro").OrExpr(When("user.tier").Eq("vip")), `(plan == "pro") or (user.tier == "vip")`},
	}
	for _, tt := range checks {
		t.Run(tt.name, func(t *testing.T) {
			if tt.b.String() != tt.want {
				t.Fatalf("builder string = %q, want %q", tt.b.String(), tt.want)
			}
		})
	}

	b := When("user.age").Between(18, 65).
		And("user.country").InPath("allowed.countries").
		AndVar("minScore").LtePath("score").
		AndExpr(When("user.email").Exists()).
		And("user.nickname").IsNull()
	expr, err := b.Compile(WithVariables(MapFacts{"minScore": 70}))
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}
	matched, err := expr.EvalBool(context.Background(), MapFacts{
		"user": map[string]any{"age": 34, "country": "NP", "email": "sujit@example.com", "nickname": nil},
		"allowed": map[string]any{
			"countries": []string{"NP", "US"},
		},
		"score": 91,
	})
	if err != nil {
		t.Fatalf("EvalBool returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected ergonomic builder expression to match")
	}
}

func TestEngineRulesGroupsAndWorkflow(t *testing.T) {
	engine := NewEngine(
		WithExpressionOptions(WithTrace(true)),
		WithActionHandler("notify", func(_ context.Context, a Action, _ Facts) (Event, error) {
			return Event{Type: "handled", Payload: a.Payload}, nil
		}),
	)
	enabled := true
	rs := RuleSet{
		Name: "checkout",
		Rules: []Rule{
			{
				ID:        "minor",
				Priority:  1,
				Condition: `user.age < 18`,
				Decision:  "deny",
			},
			{
				ID:       "vip",
				Priority: 10,
				Enabled:  &enabled,
				Group: &Group{
					Mode: GroupAll,
					Rules: []Rule{
						{ID: "adult", Condition: `user.age >= 18`},
						{ID: "tier", Condition: `user.tier == "vip"`},
					},
				},
				Score:       7,
				Decision:    "approve",
				Actions:     []Action{{Type: "notify", Payload: map[string]any{"channel": "risk"}}},
				StopOnMatch: true,
			},
		},
		Workflows: []Workflow{
			{
				Name:       "review",
				StartStage: "start",
				Stages: []Stage{
					{
						Name: "start",
						Rules: []Rule{{
							ID:        "route",
							Condition: `order.total > 100`,
							Decision:  "manual_review",
							NextStage: "finish",
						}},
					},
					{
						Name: "finish",
						Rules: []Rule{{
							ID:        "finish",
							Condition: `true`,
							Events:    []Event{{Type: "workflow_finished"}},
						}},
					},
				},
			},
		},
	}
	if err := engine.AddRuleSet(rs); err != nil {
		t.Fatalf("AddRuleSet returned error: %v", err)
	}
	res, err := engine.Evaluate(context.Background(), MapFacts{
		"user":  map[string]any{"age": 25, "tier": "vip"},
		"order": map[string]any{"total": 150},
	}, "checkout")
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !res.Matched || res.Decision != "manual_review" || res.Score != 7 {
		t.Fatalf("unexpected result: %#v", res)
	}
	if len(res.Actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(res.Actions))
	}
	if len(res.Events) < 2 {
		t.Fatalf("expected handler and workflow events, got %#v", res.Events)
	}
}

func TestEngineMatchedRuleIDsValidityAndOrdering(t *testing.T) {
	engine := NewEngine()
	now := time.Now().Unix()
	rs := RuleSet{
		Name: "parity",
		Rules: []Rule{
			{ID: "1", Priority: 10, Salience: 1, Condition: `true`, ValidUntil: now - 1, Decision: "deny"},
			{
				ID:       "2",
				Priority: 20,
				Salience: 1,
				Group: &Group{
					Mode: GroupAll,
					Rules: []Rule{
						{ID: "3", Condition: `user.age >= 18`},
						{ID: "4", Condition: `user.tier == "vip"`},
					},
				},
				Decision: "allow",
			},
			{ID: "5", Priority: 30, Salience: 1, Condition: `true`, ValidFrom: now + 60, Decision: "deny"},
		},
		Workflows: []Workflow{{
			Name:       "wf",
			StartStage: "start",
			Stages: []Stage{{
				Name: "start",
				Rules: []Rule{{
					ID:        "6",
					Condition: `order.total > 100`,
					Events:    []Event{{Type: "workflow_matched"}},
				}},
			}},
		}},
	}
	if err := engine.AddRuleSet(rs); err != nil {
		t.Fatalf("AddRuleSet returned error: %v", err)
	}
	res, err := engine.Evaluate(context.Background(), MapFacts{
		"user":  map[string]any{"age": 29, "tier": "vip"},
		"order": map[string]any{"total": 150},
	}, "parity")
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !res.Matched || !res.Allowed {
		t.Fatalf("unexpected result: %#v", res)
	}
	if got, want := res.MatchedRuleIDs, []RuleID{2, 6}; !reflect.DeepEqual(got, want) {
		t.Fatalf("matched rule ids = %#v, want %#v", got, want)
	}
	if len(res.Events) != 1 || res.Events[0].Type != "workflow_matched" {
		t.Fatalf("events = %#v", res.Events)
	}
}

func TestEngineExecutionModes(t *testing.T) {
	run := func(t *testing.T, mode ExecutionMode, rules []Rule) Result {
		t.Helper()
		engine := NewEngine()
		if err := engine.AddRuleSet(RuleSet{Name: "modes", ExecutionMode: mode, Rules: rules}); err != nil {
			t.Fatalf("AddRuleSet returned error: %v", err)
		}
		res, err := engine.Evaluate(context.Background(), MapFacts{}, "modes")
		if err != nil {
			t.Fatalf("Evaluate returned error: %v", err)
		}
		return res
	}

	t.Run("highest-priority-uses-salience", func(t *testing.T) {
		res := run(t, HighestPriority, []Rule{
			{ID: "1", Priority: 10, Salience: 1, Condition: `true`, Decision: "deny"},
			{ID: "2", Priority: 10, Salience: 5, Condition: `true`, Decision: "allow"},
			{ID: "3", Priority: 10, Salience: 3, Condition: `true`, Decision: "manual"},
		})
		if got, want := res.MatchedRuleIDs, []RuleID{2}; !reflect.DeepEqual(got, want) || !res.Allowed {
			t.Fatalf("result = %#v, want ids %#v and allowed", res, want)
		}
	})

	t.Run("deny-overrides", func(t *testing.T) {
		res := run(t, DenyOverrides, []Rule{
			{ID: "1", Priority: 3, Condition: `true`, Decision: "allow"},
			{ID: "2", Priority: 2, Condition: `true`, Actions: []Action{{Type: "deny"}}},
			{ID: "3", Priority: 1, Condition: `true`, Decision: "allow"},
		})
		if got, want := res.MatchedRuleIDs, []RuleID{1, 2}; !reflect.DeepEqual(got, want) || res.Allowed {
			t.Fatalf("result = %#v, want ids %#v and denied", res, want)
		}
	})

	t.Run("allow-overrides", func(t *testing.T) {
		res := run(t, AllowOverrides, []Rule{
			{ID: "1", Priority: 3, Condition: `true`, Decision: "deny"},
			{ID: "2", Priority: 2, Condition: `true`, Actions: []Action{{Type: "allow"}}},
			{ID: "3", Priority: 1, Condition: `true`, Decision: "deny"},
		})
		if got, want := res.MatchedRuleIDs, []RuleID{1, 2}; !reflect.DeepEqual(got, want) || !res.Allowed {
			t.Fatalf("result = %#v, want ids %#v and allowed", res, want)
		}
	})

	t.Run("score-based-additive", func(t *testing.T) {
		res := run(t, ScoreBased, []Rule{
			{ID: "1", Priority: 2, Condition: `true`, Score: 1},
			{ID: "2", Priority: 1, Condition: `true`, Score: 2},
		})
		if got, want := res.MatchedRuleIDs, []RuleID{1, 2}; !reflect.DeepEqual(got, want) || res.Score != 3 {
			t.Fatalf("result = %#v, want ids %#v and score 3", res, want)
		}
	})

	t.Run("first-match-internal-mode", func(t *testing.T) {
		engine := NewEngine()
		rules, err := engine.compileRules([]Rule{
			{ID: "1", Priority: 2, Condition: `true`, Decision: "allow"},
			{ID: "2", Priority: 1, Condition: `true`, Decision: "deny"},
		})
		if err != nil {
			t.Fatalf("compileRules returned error: %v", err)
		}
		sortRules(rules)
		res, err := engine.evalRules(context.Background(), MapFacts{}, rules, FirstMatch)
		if err != nil {
			t.Fatalf("evalRules returned error: %v", err)
		}
		if got, want := res.MatchedRuleIDs, []RuleID{1}; !reflect.DeepEqual(got, want) || !res.Allowed {
			t.Fatalf("result = %#v, want ids %#v and allowed", res, want)
		}
	})
}

func TestRuleSetJSONRoundTrip(t *testing.T) {
	rs := RuleSet{
		Name: "json",
		Rules: []Rule{{
			ID:        "r1",
			Condition: `user.age >= 18`,
			Actions:   []Action{{Type: "allow"}},
		}},
	}
	data, err := rs.JSON()
	if err != nil {
		t.Fatalf("JSON returned error: %v", err)
	}
	var decoded RuleSet
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	engine := NewEngine()
	if err := engine.AddRuleSet(decoded); err != nil {
		t.Fatalf("AddRuleSet returned error: %v", err)
	}
	res, err := engine.Evaluate(context.Background(), MapFacts{"user": map[string]any{"age": 18}}, "json")
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !res.Matched || len(res.Actions) != 1 {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestCompiledExpressionConcurrentUse(t *testing.T) {
	expr := MustCompile(`user.age >= 18 and "vip" in tags`)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				res, err := expr.Eval(context.Background(), MapFacts{
					"user": map[string]any{"age": 30},
					"tags": []string{"vip"},
				})
				if err != nil || !res.Matched {
					t.Errorf("Eval = %#v, %v", res, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
