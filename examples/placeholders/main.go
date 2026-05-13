package main

import (
	"context"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func main() {
	facts := condition.MapFacts{
		"user":  map[string]any{"age": 29, "country": "NP"},
		"order": map[string]any{"total": 149.90},
		"tags":  []string{"vip", "returning"},
	}

	vars := condition.MapFacts{
		"minAge":   18,
		"minTotal": 100,
		"required": map[string]any{
			"country": "NP",
			"tag":     "vip",
		},
	}

	expr := condition.MustCompile(
		`{{minAge}} <= user.age and ${minTotal} <= order.total and {{required.country}} == user.country and {{required.tag}} in tags`,
		condition.WithVariables(vars),
	)

	matched, err := expr.EvalBool(context.Background(), facts)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("matched:", matched)

	compiledThresholds := condition.MustCompile(
		`user.age >= {{minAge}} and user.country in {{countries}}`,
		condition.WithInterpolationMap(map[string]any{
			"minAge":    18,
			"countries": []string{"NP", "US"},
		}),
	)
	matched, err = compiledThresholds.EvalBool(context.Background(), facts)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("interpolated:", matched)
}
