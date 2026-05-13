package main

import (
	"context"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func main() {
	facts := condition.MapFacts{
		"user": map[string]any{"tier": "enterprise"},
		"messages": []any{
			map[string]any{"country": "NP", "category": "otp", "cost": 0.010, "success_rate": 0.991},
			map[string]any{"country": "NP", "category": "otp", "cost": 0.008, "success_rate": 0.985},
			map[string]any{"country": "US", "category": "alert", "cost": 0.012, "success_rate": 0.970},
		},
	}

	expr := condition.MustCompile(`
		user.tier == "enterprise" and
		count(messages) >= 3 and
		avg(messages, "success_rate") >= 0.98 and
		sum(messages, "cost") < 0.05 and
		any(messages, "country", "==", "NP") and
		none(messages, "success_rate", "<", 0.95) and
		get(first(sortBy(messages, "cost")), "cost") == 0.008 and
		sum(take(sortBy(messages, "success_rate", "desc"), 2), "success_rate") > 1.97 and
		join(distinct(messages, "country"), "|") == "NP|US" and
		groupCount(messages, "country", "NP") == 2 and
		groupAvg(messages, "country", "NP", "success_rate") > 0.98 and
		distinctCount(messages, "country") == 2
	`)

	matched, err := expr.EvalBool(context.Background(), facts)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("matched:", matched)
}
