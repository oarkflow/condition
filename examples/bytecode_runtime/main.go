package main

import (
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func main() {
	registry := condition.NewRegistry()

	program, err := condition.CompileRuleSet(condition.RuleSet{
		Name:          "workflow.approval",
		ExecutionMode: condition.FirstMatch,
		Rules: []condition.Rule{
			{
				ID:        "1001",
				Priority:  100,
				Condition: `amount > 100000 and department in ["finance", "procurement"] and risk_score >= 70`,
				Decision:  "require_approval",
				Score:     10,
				Actions: []condition.Action{
					{Type: "notify", Payload: map[string]any{"channel": "email"}},
				},
			},
		},
	}, registry)
	if err != nil {
		log.Fatal(err)
	}

	facts := condition.NewTypedFacts(registry.Field("risk_score"))
	facts.SetInt(registry.Field("amount"), 150000)
	facts.SetString(registry.Field("department"), registry.Intern("finance"))
	facts.SetInt(registry.Field("risk_score"), 75)

	ctx := condition.EvalContext{
		Matched: make([]condition.RuleID, 0, 4),
		Actions: make([]condition.CompiledAction, 0, 4),
		Stack:   make([]bool, 0, 8),
	}

	runtime := condition.NewRuntime(program)
	result := runtime.Evaluate(registry.Namespace("workflow.approval"), facts, &ctx)

	fmt.Printf("matched=%v rules=%v decision=%s score=%d actions=%d\n",
		result.Matched,
		result.MatchedRuleIDs,
		result.Decision,
		result.ScoreValue,
		len(result.CompiledActions),
	)

	anyResult, err := runtime.EvaluateAny(registry.Namespace("workflow.approval"), map[string]any{
		"amount":     150000,
		"department": "finance",
		"risk_score": 75,
	}, &ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("map facts matched=%v decision=%s\n", anyResult.Matched, anyResult.Decision)
}
