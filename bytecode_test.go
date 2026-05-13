package condition

import (
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func TestRegistryAndTypedFacts(t *testing.T) {
	reg := NewRegistry()
	amount := reg.Field("amount")
	dept := reg.Field("department")
	if amount == 0 || dept == 0 || amount == dept {
		t.Fatalf("field ids amount=%d department=%d", amount, dept)
	}
	facts := NewTypedFacts(dept)
	facts.SetInt(amount, 150000)
	facts.SetString(dept, reg.Intern("finance"))
	if facts.kind(amount) != ValueInt || facts.ints[amount] != 150000 {
		t.Fatalf("amount fact not stored")
	}
	if facts.kind(dept) != ValueString || reg.String(facts.strings[dept]) != "finance" {
		t.Fatalf("department fact not stored")
	}
}

func TestCompileDSLEvalBoolBytecode(t *testing.T) {
	reg := NewRegistry()
	expr, err := CompileDSL(`amount > 100000 and department in ["finance", "procurement"] and risk_score >= 70`, reg)
	if err != nil {
		t.Fatalf("CompileDSL returned error: %v", err)
	}
	facts := NewTypedFacts(reg.Field("risk_score"))
	facts.SetInt(reg.Field("amount"), 150000)
	facts.SetString(reg.Field("department"), reg.Intern("finance"))
	facts.SetInt(reg.Field("risk_score"), 72)
	ctx := EvalContext{Stack: make([]bool, 0, 8)}
	if !expr.EvalBool(facts, &ctx) {
		t.Fatal("expected bytecode expression to match")
	}
	facts.SetString(reg.Field("department"), reg.Intern("sales"))
	if expr.EvalBool(facts, &ctx) {
		t.Fatal("expected bytecode expression to reject")
	}
}

func TestBytecodeCompileTimeInterpolation(t *testing.T) {
	reg := NewRegistry()
	expr, err := CompileDSL(
		`amount > {{minAmount}} and department in {{departments}}`,
		reg,
		WithInterpolationMap(map[string]any{
			"minAmount":   100000,
			"departments": []string{"finance", "procurement"},
		}),
	)
	if err != nil {
		t.Fatalf("CompileDSL returned error: %v", err)
	}
	facts := NewTypedFacts(reg.Field("department"))
	facts.SetInt(reg.Field("amount"), 150000)
	facts.SetString(reg.Field("department"), reg.Intern("finance"))
	ctx := EvalContext{Stack: make([]bool, 0, 4)}
	if !expr.EvalBool(facts, &ctx) {
		t.Fatal("expected interpolated bytecode expression to match")
	}

	program, err := CompileRuleSet(RuleSet{
		Name: "interpolated",
		Rules: []Rule{{
			ID:        "1",
			Condition: `amount > {{minAmount}} and department == {{department}}`,
			Decision:  "allow",
		}},
	}, reg, WithCompileExpressionOptions(WithInterpolationMap(map[string]any{
		"minAmount":  100000,
		"department": "finance",
	})))
	if err != nil {
		t.Fatalf("CompileRuleSet returned error: %v", err)
	}
	runtime := NewRuntime(program)
	ctx = EvalContext{Matched: make([]RuleID, 0, 2), Actions: make([]CompiledAction, 0, 2), Stack: make([]bool, 0, 4)}
	res := runtime.Evaluate(reg.Namespace("interpolated"), facts, &ctx)
	if !res.Matched || !res.Allowed {
		t.Fatalf("expected interpolated bytecode ruleset to allow, got %#v", res)
	}
}

func TestProgramEvaluateAndRuntimeReload(t *testing.T) {
	reg := NewRegistry()
	rs := RuleSet{
		Name:          "workflow.approval",
		ExecutionMode: FirstMatch,
		Rules: []Rule{
			{
				ID:        "1001",
				Priority:  100,
				Condition: `amount > 100000 and department in ["finance", "procurement"] and risk_score >= 70`,
				Decision:  "require_approval",
				Actions:   []Action{{Type: "notify", Payload: map[string]any{"channel": "email"}}},
				Score:     5,
			},
		},
	}
	program, err := CompileRuleSet(rs, reg)
	if err != nil {
		t.Fatalf("CompileRuleSet returned error: %v", err)
	}
	runtime := NewRuntime(program)
	facts := NewTypedFacts(reg.Field("risk_score"))
	facts.SetInt(reg.Field("amount"), 150000)
	facts.SetString(reg.Field("department"), reg.Intern("finance"))
	facts.SetInt(reg.Field("risk_score"), 75)
	ctx := EvalContext{
		Matched: make([]RuleID, 0, 4),
		Actions: make([]CompiledAction, 0, 4),
		Stack:   make([]bool, 0, 8),
	}
	res := runtime.Evaluate(reg.Namespace("workflow.approval"), facts, &ctx)
	if !res.Matched || len(res.MatchedRuleIDs) != 1 || res.MatchedRuleIDs[0] != 1001 {
		t.Fatalf("matched rules = %#v", res.MatchedRuleIDs)
	}
	if res.Decision != "require_approval" || res.ScoreValue != 5 || len(res.CompiledActions) != 2 {
		t.Fatalf("result = %#v", res)
	}

	empty, err := CompileRuleSet(RuleSet{Name: "workflow.approval"}, reg)
	if err != nil {
		t.Fatalf("CompileRuleSet empty returned error: %v", err)
	}
	runtime.Reload(empty)
	res = runtime.Evaluate(reg.Namespace("workflow.approval"), facts, &ctx)
	if res.Matched {
		t.Fatal("expected no match after reload")
	}
}

func TestMapAndStructFactsAdapters(t *testing.T) {
	reg := NewRegistry()
	facts := NewTypedFacts(0)
	err := MapFactsAdapter(reg, map[string]any{
		"user":   map[string]any{"role": "manager"},
		"amount": 120000,
	}, facts)
	if err != nil {
		t.Fatalf("MapFactsAdapter returned error: %v", err)
	}
	if facts.strings[reg.Field("user.role")] != reg.Intern("manager") {
		t.Fatalf("map adapter did not set nested string")
	}

	type Request struct {
		Amount int `json:"amount"`
		User   struct {
			Role string `json:"role"`
		} `json:"user"`
	}
	facts.Reset()
	req := Request{Amount: 100}
	req.User.Role = "manager"
	if err := StructFactsAdapter(reg, req, facts); err != nil {
		t.Fatalf("StructFactsAdapter returned error: %v", err)
	}
	if facts.ints[reg.Field("amount")] != 100 || facts.strings[reg.Field("user.role")] != reg.Intern("manager") {
		t.Fatalf("struct adapter facts mismatch")
	}
}

func TestEvaluateAnySupportsMapStructAndSlices(t *testing.T) {
	reg := NewRegistry()
	program, err := CompileRuleSet(RuleSet{
		Name: "mixed",
		Rules: []Rule{
			{ID: "1", Condition: `user.role == "manager" and amount > 100`, Decision: "allow"},
			{ID: "2", Condition: `orders[0].amount > 50 and orders[1].country == "NP"`, Decision: "allow"},
			{ID: "3", Condition: `items[0].amount > 10 and items[1].country == "NP"`, Decision: "allow"},
		},
	}, reg)
	if err != nil {
		t.Fatalf("CompileRuleSet returned error: %v", err)
	}
	ctx := EvalContext{
		Matched:    make([]RuleID, 0, 4),
		Actions:    make([]CompiledAction, 0, 4),
		Stack:      make([]bool, 0, 8),
		Candidates: make([]int, 0, 8),
		TypedFacts: NewTypedFacts(reg.MaxField()),
	}
	res, err := program.EvaluateAny(reg.Namespace("mixed"), map[string]any{
		"user":   map[string]any{"role": "manager"},
		"amount": 150,
	}, &ctx)
	if err != nil {
		t.Fatalf("EvaluateAny map returned error: %v", err)
	}
	if !res.Matched || res.MatchedRuleIDs[0] != 1 {
		t.Fatalf("map result = %#v", res)
	}

	type User struct {
		Role string `json:"role"`
	}
	type Request struct {
		User   User `json:"user"`
		Amount int  `json:"amount"`
	}
	res, err = program.EvaluateAny(reg.Namespace("mixed"), Request{User: User{Role: "manager"}, Amount: 150}, &ctx)
	if err != nil {
		t.Fatalf("EvaluateAny struct returned error: %v", err)
	}
	if !res.Matched || res.MatchedRuleIDs[0] != 1 {
		t.Fatalf("struct result = %#v", res)
	}

	res, err = program.EvaluateAny(reg.Namespace("mixed"), map[string]any{
		"orders": []map[string]any{
			{"amount": 75, "country": "US"},
			{"amount": 10, "country": "NP"},
		},
	}, &ctx)
	if err != nil {
		t.Fatalf("EvaluateAny slice map field returned error: %v", err)
	}
	if !res.Matched || res.MatchedRuleIDs[0] != 2 {
		t.Fatalf("slice map field result = %#v", res)
	}

	type Order struct {
		Amount  int    `json:"amount"`
		Country string `json:"country"`
	}
	res, err = program.EvaluateAny(reg.Namespace("mixed"), []Order{
		{Amount: 75, Country: "US"},
		{Amount: 10, Country: "NP"},
	}, &ctx)
	if err != nil {
		t.Fatalf("EvaluateAny root slice struct returned error: %v", err)
	}
	if !res.Matched || res.MatchedRuleIDs[0] != 3 {
		t.Fatalf("root slice struct result = %#v", res)
	}
}

func TestCompileDSLEvalAny(t *testing.T) {
	reg := NewRegistry()
	expr, err := CompileDSL(`user.role == "manager" and approvals[0].level >= 2`, reg)
	if err != nil {
		t.Fatalf("CompileDSL returned error: %v", err)
	}
	ctx := EvalContext{Stack: make([]bool, 0, 4), TypedFacts: NewTypedFacts(reg.MaxField())}
	ok, err := expr.EvalAny(map[string]any{
		"user":      map[string]string{"role": "manager"},
		"approvals": []map[string]any{{"level": 2}},
	}, &ctx)
	if err != nil {
		t.Fatalf("EvalAny returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected EvalAny to match")
	}
}

func TestRuntimeConcurrentEvaluate(t *testing.T) {
	reg := NewRegistry()
	program, err := CompileRuleSet(RuleSet{
		Name: "auth.access",
		Rules: []Rule{{
			ID:        "1",
			Condition: `user.role == "manager" and resource.department == "finance"`,
			Decision:  "allow",
		}},
	}, reg)
	if err != nil {
		t.Fatalf("CompileRuleSet returned error: %v", err)
	}
	runtime := NewRuntime(program)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			facts := NewTypedFacts(reg.Field("resource.department"))
			facts.SetString(reg.Field("user.role"), reg.Intern("manager"))
			facts.SetString(reg.Field("resource.department"), reg.Intern("finance"))
			ctx := EvalContext{Matched: make([]RuleID, 0, 2), Actions: make([]CompiledAction, 0, 2), Stack: make([]bool, 0, 4)}
			for j := 0; j < 100; j++ {
				res := runtime.Evaluate(reg.Namespace("auth.access"), facts, &ctx)
				if !res.Matched || !res.Allowed {
					t.Errorf("expected allow match, got %#v", res)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestBytecodeEqualityIndexSkipsNonMatchingRules(t *testing.T) {
	reg := NewRegistry()
	rules := make([]Rule, 100)
	for i := range rules {
		country := "US"
		if i == 42 {
			country = "NP"
		}
		rules[i] = Rule{
			ID:        strconv.Itoa(i + 1),
			Condition: `country == "` + country + `" and amount > 100`,
			Decision:  "allow",
		}
	}
	program, err := CompileRuleSet(RuleSet{Name: "routing", Rules: rules}, reg)
	if err != nil {
		t.Fatalf("CompileRuleSet returned error: %v", err)
	}
	facts := NewTypedFacts(reg.Field("amount"))
	facts.SetString(reg.Field("country"), reg.Intern("NP"))
	facts.SetInt(reg.Field("amount"), 150)
	ctx := EvalContext{Matched: make([]RuleID, 0, 4), Actions: make([]CompiledAction, 0, 4), Stack: make([]bool, 0, 4), Candidates: make([]int, 0, 8)}
	res := program.Evaluate(reg.Namespace("routing"), facts, &ctx)
	if !res.Matched || len(res.MatchedRuleIDs) != 1 || res.MatchedRuleIDs[0] != 43 {
		t.Fatalf("result = %#v", res)
	}
	if len(ctx.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1 indexed candidate", len(ctx.Candidates))
	}
}

func TestBytecodeExecutionModeParity(t *testing.T) {
	evaluate := func(t *testing.T, mode ExecutionMode, rules []Rule, opts ...CompileOption) Result {
		t.Helper()
		reg := NewRegistry()
		program, err := CompileRuleSet(RuleSet{Name: "modes", ExecutionMode: mode, Rules: rules}, reg, opts...)
		if err != nil {
			t.Fatalf("CompileRuleSet returned error: %v", err)
		}
		ctx := EvalContext{Matched: make([]RuleID, 0, 4), Actions: make([]CompiledAction, 0, 4), Stack: make([]bool, 0, 4)}
		return program.Evaluate(reg.Namespace("modes"), NewTypedFacts(0), &ctx)
	}

	t.Run("first-match-option", func(t *testing.T) {
		res := evaluate(t, FirstMatch, []Rule{
			{ID: "1", Priority: 2, Condition: `true`, Decision: "allow"},
			{ID: "2", Priority: 1, Condition: `true`, Decision: "deny"},
		}, WithExecutionMode(FirstMatch))
		if got, want := res.MatchedRuleIDs, []RuleID{1}; !reflect.DeepEqual(got, want) || !res.Allowed {
			t.Fatalf("result = %#v, want ids %#v and allowed", res, want)
		}
	})

	t.Run("highest-priority-uses-salience", func(t *testing.T) {
		res := evaluate(t, HighestPriority, []Rule{
			{ID: "1", Priority: 10, Salience: 1, Condition: `true`, Decision: "deny"},
			{ID: "2", Priority: 10, Salience: 5, Condition: `true`, Decision: "allow"},
			{ID: "3", Priority: 10, Salience: 3, Condition: `true`, Decision: "deny"},
		})
		if got, want := res.MatchedRuleIDs, []RuleID{2}; !reflect.DeepEqual(got, want) || !res.Allowed {
			t.Fatalf("result = %#v, want ids %#v and allowed", res, want)
		}
	})

	t.Run("deny-overrides", func(t *testing.T) {
		res := evaluate(t, DenyOverrides, []Rule{
			{ID: "1", Priority: 3, Condition: `true`, Decision: "allow"},
			{ID: "2", Priority: 2, Condition: `true`, Actions: []Action{{Type: "deny"}}},
			{ID: "3", Priority: 1, Condition: `true`, Decision: "allow"},
		})
		if got, want := res.MatchedRuleIDs, []RuleID{1, 2}; !reflect.DeepEqual(got, want) || res.Allowed {
			t.Fatalf("result = %#v, want ids %#v and denied", res, want)
		}
	})

	t.Run("allow-overrides", func(t *testing.T) {
		res := evaluate(t, AllowOverrides, []Rule{
			{ID: "1", Priority: 3, Condition: `true`, Decision: "deny"},
			{ID: "2", Priority: 2, Condition: `true`, Actions: []Action{{Type: "allow"}}},
			{ID: "3", Priority: 1, Condition: `true`, Decision: "deny"},
		})
		if got, want := res.MatchedRuleIDs, []RuleID{1, 2}; !reflect.DeepEqual(got, want) || !res.Allowed {
			t.Fatalf("result = %#v, want ids %#v and allowed", res, want)
		}
	})

	t.Run("score-based-additive", func(t *testing.T) {
		res := evaluate(t, ScoreBased, []Rule{
			{ID: "1", Priority: 2, Condition: `true`, Score: 1},
			{ID: "2", Priority: 1, Condition: `true`, Score: 2},
		})
		if got, want := res.MatchedRuleIDs, []RuleID{1, 2}; !reflect.DeepEqual(got, want) || res.Score != 3 {
			t.Fatalf("result = %#v, want ids %#v and score 3", res, want)
		}
	})
}

func TestBytecodeAndEngineSimpleParity(t *testing.T) {
	rules := RuleSet{
		Name:          "approval",
		ExecutionMode: HighestPriority,
		Rules: []Rule{
			{ID: "1", Priority: 10, Salience: 1, Condition: `amount > 100`, Decision: "deny", Score: 1},
			{ID: "2", Priority: 10, Salience: 5, Condition: `amount > 100`, Decision: "allow", Score: 2},
		},
	}
	engine := NewEngine()
	if err := engine.AddRuleSet(rules); err != nil {
		t.Fatalf("AddRuleSet returned error: %v", err)
	}
	engineResult, err := engine.Evaluate(nil, MapFacts{"amount": 150}, "approval")
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	reg := NewRegistry()
	program, err := CompileRuleSet(rules, reg)
	if err != nil {
		t.Fatalf("CompileRuleSet returned error: %v", err)
	}
	facts := NewTypedFacts(reg.Field("amount"))
	facts.SetInt(reg.Field("amount"), 150)
	ctx := EvalContext{Matched: make([]RuleID, 0, 2), Actions: make([]CompiledAction, 0, 2), Stack: make([]bool, 0, 4)}
	bytecodeResult := program.Evaluate(reg.Namespace("approval"), facts, &ctx)

	if !reflect.DeepEqual(bytecodeResult.MatchedRuleIDs, engineResult.MatchedRuleIDs) ||
		bytecodeResult.Decision != engineResult.Decision ||
		bytecodeResult.Allowed != engineResult.Allowed ||
		bytecodeResult.Score != engineResult.Score {
		t.Fatalf("bytecode = %#v, engine = %#v", bytecodeResult, engineResult)
	}
}

func BenchmarkBytecodeSingleComparison(b *testing.B) {
	reg := NewRegistry()
	expr, err := CompileDSL(`amount > 100000`, reg)
	if err != nil {
		b.Fatal(err)
	}
	facts := NewTypedFacts(reg.Field("amount"))
	facts.SetInt(reg.Field("amount"), 150000)
	ctx := EvalContext{Stack: make([]bool, 0, 4)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !expr.EvalBool(facts, &ctx) {
			b.Fatal("expected match")
		}
	}
}

func BenchmarkBytecodeBooleanExpression(b *testing.B) {
	reg := NewRegistry()
	expr, err := CompileDSL(`amount > 100000 and department in ["finance", "procurement"] and risk_score >= 70`, reg)
	if err != nil {
		b.Fatal(err)
	}
	facts := NewTypedFacts(reg.Field("risk_score"))
	facts.SetInt(reg.Field("amount"), 150000)
	facts.SetString(reg.Field("department"), reg.Intern("finance"))
	facts.SetInt(reg.Field("risk_score"), 72)
	ctx := EvalContext{Stack: make([]bool, 0, 8)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !expr.EvalBool(facts, &ctx) {
			b.Fatal("expected match")
		}
	}
}

func BenchmarkBytecodeProgram25Rules(b *testing.B) {
	benchmarkBytecodeProgram(b, 25)
}

func BenchmarkBytecodeProgram100Rules(b *testing.B) {
	benchmarkBytecodeProgram(b, 100)
}

func BenchmarkBytecodeProgram1000RulesIndexedSparse(b *testing.B) {
	reg := NewRegistry()
	rules := make([]Rule, 1000)
	for i := range rules {
		country := "US"
		if i == 777 {
			country = "NP"
		}
		rules[i] = Rule{
			ID:        strconv.Itoa(i + 1),
			Condition: `country == "` + country + `" and amount > 100000 and risk_score >= 70`,
			Decision:  "allow",
		}
	}
	program, err := CompileRuleSet(RuleSet{Name: "routing", Rules: rules}, reg)
	if err != nil {
		b.Fatal(err)
	}
	facts := NewTypedFacts(reg.Field("risk_score"))
	facts.SetString(reg.Field("country"), reg.Intern("NP"))
	facts.SetInt(reg.Field("amount"), 150000)
	facts.SetInt(reg.Field("risk_score"), 75)
	ctx := EvalContext{Matched: make([]RuleID, 0, 4), Actions: make([]CompiledAction, 0, 4), Stack: make([]bool, 0, 8), Candidates: make([]int, 0, 8)}
	ns := reg.Namespace("routing")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := program.Evaluate(ns, facts, &ctx)
		if !res.Matched {
			b.Fatal("expected match")
		}
	}
}

func benchmarkBytecodeProgram(b *testing.B, n int) {
	reg := NewRegistry()
	rules := make([]Rule, n)
	for i := range rules {
		rules[i] = Rule{
			ID:        strconv.Itoa(i + 1),
			Priority:  i,
			Condition: `amount > 100000 and department == "finance" and risk_score >= 70`,
			Decision:  "allow",
		}
	}
	program, err := CompileRuleSet(RuleSet{Name: "bench", Rules: rules}, reg)
	if err != nil {
		b.Fatal(err)
	}
	facts := NewTypedFacts(reg.Field("risk_score"))
	facts.SetInt(reg.Field("amount"), 150000)
	facts.SetString(reg.Field("department"), reg.Intern("finance"))
	facts.SetInt(reg.Field("risk_score"), 72)
	ctx := EvalContext{Matched: make([]RuleID, 0, n), Actions: make([]CompiledAction, 0, n), Stack: make([]bool, 0, 8)}
	ns := reg.Namespace("bench")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := program.Evaluate(ns, facts, &ctx)
		if !res.Matched {
			b.Fatal("expected match")
		}
	}
}
