package main

import (
	"context"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func main() {
	eligibility := condition.
		When("user.age").Gte(18).
		And("country").In("US", "CA", "NP").
		And("plan").Ne("free").
		And("score").BetweenPath("limits.min_score", "limits.max_score")

	override := condition.
		When("user.email").Exists().
		AndVar("manualOverride").Eq(true)

	builder := eligibility.
		AndExpr(condition.When("country").InPath("allowed.countries")).
		OrExpr(override)

	fmt.Println("compiled DSL:", builder.String())

	expr, err := builder.Compile(condition.WithVariables(condition.MapFacts{
		"manualOverride": false,
	}))
	if err != nil {
		log.Fatal(err)
	}

	result, err := expr.Eval(context.Background(), condition.MapFacts{
		"user":    map[string]any{"age": 34},
		"country": "NP",
		"plan":    "pro",
		"score":   92,
		"limits":  map[string]any{"min_score": 70, "max_score": 100},
		"allowed": map[string]any{"countries": []string{"NP", "US"}},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("matched:", result.Matched)
}
