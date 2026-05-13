package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func main() {
	facts := condition.MapFacts{
		"user": map[string]any{
			"age":   29,
			"tier":  "vip",
			"email": "sujit@example.com",
		},
		"order": map[string]any{
			"total": 149.90,
		},
		"tags": []string{"vip", "returning"},
	}

	expr := `user.age >= 18 and user.tier == "vip" and order.total > 100 and "@" in user.email`
	compiled, err := condition.Compile(expr, condition.WithTrace(true))
	if err != nil {
		log.Fatal(err)
	}

	result, err := compiled.Eval(context.Background(), facts)
	if err != nil {
		log.Fatal(err)
	}

	printJSON("simple expression result", result)
}

func printJSON(title string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s:\n%s\n", title, data)
}
