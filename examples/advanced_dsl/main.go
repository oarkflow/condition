package main

import (
	"context"
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

func main() {
	facts := condition.MapFacts{
		"user": map[string]any{
			"tier":       " VIP ",
			"email":      "Sujit@Example.com",
			"created_at": "2026-01-15T09:00:00Z",
		},
		"order": map[string]any{
			"total":      145.75,
			"discount":   nil,
			"created_at": "2026-05-01T10:30:00Z",
		},
		"route": map[string]any{
			"user_id":           nil,
			"recipient_country": "NP",
			"min_message_count": 1,
			"max_message_count": 1000,
			"status":            "active",
			"valid_from":        nil,
			"valid_to":          nil,
		},
		"recipient": map[string]any{"country": "NP"},
		"message":   map[string]any{"count": 250},
		"orders": []any{
			map[string]any{"status": "paid", "amount": 120, "risk": 0.05},
			map[string]any{"status": "paid", "amount": 80, "risk": 0.10},
			map[string]any{"status": "failed", "amount": 25, "risk": 0.40},
		},
	}

	expressions := []string{
		`lower(trim(user.tier)) == "vip"`,
		`defaultValue(order.discount, 0) == 0 and between(order.total, 100, 200)`,
		`before(user.created_at, now()) and age(user.created_at, "day") >= 0`,
		`active(route.status) and validNow(route.valid_from, route.valid_to)`,
		`nullableMatch(route.user_id, user.id) and nullableMatch(route.recipient_country, recipient.country)`,
		`rangeMatch(route.min_message_count, route.max_message_count, message.count)`,
		`sum(filter(orders, "status", "==", "paid"), "amount") >= 200`,
		`groupCountWhere(orders, "status", "paid", "amount", ">", 100) == 1`,
		`get(top(orders, "amount"), "amount") == 120`,
	}

	for _, dsl := range expressions {
		matched, err := condition.MustCompile(dsl).EvalBool(context.Background(), facts)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-90s => %v\n", dsl, matched)
	}
}
